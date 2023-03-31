package file

import (
	"errors"
	"fmt"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/def"
	"gitee.com/sy_183/common/flag"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lifecycle/retry"
	taskPkg "gitee.com/sy_183/common/lifecycle/task"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/storage"
	"gitee.com/sy_183/cvds-mas/storage/file/context"
	"gitee.com/sy_183/cvds-mas/storage/meta"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const ChannelModule = Module + ".channel"

const (
	// 默认覆盖周期为7天
	DefaultCover = 7 * 24 * time.Hour
	// 最小覆盖周期为1分钟
	MinCover = time.Minute

	// 文件大小默认512M
	DefaultFileSize = 512 * unit.MeBiByte
	// 文件大小最小为1M
	MinFileSize = unit.MeBiByte

	// 默认文件持续时间为30分钟
	DefaultFileDuration = 30 * time.Minute
	// 最小文件持续时间为1分钟
	MinFileDuration = time.Minute

	// 默认检查过期索引是否要删除的间隔时间为1s
	DefaultCheckDeleteInterval = time.Second
	// 最小检查过期索引是否要删除的间隔时间为10ms
	MinCheckDeleteInterval = 10 * time.Millisecond

	DefaultWriteBufferSize  = unit.MeBiByte
	MinWriteBufferSize      = 64 * unit.KiBiByte
	DefaultWriteBufferCount = 2

	// 默认最大同步请求个数为128
	DefaultMaxSyncReqs = 128
	// 最小支持最大同步请求个数为2
	MinMaxSyncReqs = 2

	// 默认最大删除任务个数为16
	DefaultMaxDeletingFiles = 16
	// 最小支持最大删除任务个数为1
	MinMaxDeletingFiles = 1
)

var ChannelNotAvailableError = errors.New("存储通道不可用")

type Channel struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	// 存储通道名称
	name string
	// 存储文件夹路径
	directory atomic.Pointer[string]
	// 存储覆盖周期
	cover atomic.Int64

	// 文件上下文，负责文件的写入
	fileCtx    *context.Context
	switchFile bool
	// 基于数据库的元数据管理器，负责元数据(包括文件信息，索引信息)的增删改查
	metaManager meta.Manager

	// 单个文件大小，超过此大小后将写入到下一个文件中
	fileSize unit.Size
	// 单个文件持续时间，超过此持续时间后将写入到下一个文件中
	fileDuration time.Duration
	// 文件命名格式
	fileNameFormatter atomic.Pointer[storage.FileNameFormatter]

	// 检查索引是否可以被删除的间隔时间
	checkDeleteInterval time.Duration

	// 写缓冲区大小，如果写入后的大小超过此大小，则在写入前先执行同步到磁盘
	writeBufferSize         uint
	writeBufferPoolProvider pool.PoolProvider[*Buffer]
	// 写入缓冲区池，用于申请写入缓冲区，如果缓冲区申请失败，则丢弃当前正在写入的数据，并生成数据被丢弃的索引
	writeBufferPool pool.Pool[*Buffer]
	// 此互斥锁用于同步写入数据、写入流丢失信息和执行同步这三个接口
	writeLock sync.Mutex

	syncReq      syncRequest
	syncReqs     *container.Queue[syncRequest]
	syncReqsLock sync.Mutex

	// 需要被删除的文件信息队列，删除任务从队列中获取这些文件信息，并删除对应的文件及文件信息
	deleteFiles     *container.Queue[*meta.File]
	deleteFilesLock sync.Mutex

	// 同步任务执行器，顺序执行同步任务
	syncTaskExecutor *taskPkg.TaskExecutor
	// 删除文件任务执行器，顺序执行文件删除任务
	deleteTaskExecutor *taskPkg.TaskExecutor

	readSessions container.SyncMap[*ReadSession, struct{}]

	log.AtomicLogger
}

func NewChannel(name string, options ...storage.Option) *Channel {
	c := &Channel{
		name:               name,
		syncTaskExecutor:   taskPkg.NewTaskExecutor(1),
		deleteTaskExecutor: taskPkg.NewTaskExecutor(1),
	}
	var metaManagerConfigOpt metaManagerConfig
	for _, option := range options {
		switch opt := option.Apply(c).(type) {
		case metaManagerConfig:
			metaManagerConfigOpt = opt
		}
	}

	c.cover.CompareAndSwap(0, int64(DefaultCover))
	if cover := c.cover.Load(); time.Duration(cover) < MinCover {
		c.cover.Store(int64(MinCover))
	}

	c.fileSize = def.SetDefault(c.fileSize, DefaultFileSize)
	if c.fileSize < MinFileSize {
		c.fileSize = MinFileSize
	}

	c.fileDuration = def.SetDefault(c.fileDuration, DefaultFileDuration)
	if c.fileDuration < MinFileDuration {
		c.fileDuration = MinFileDuration
	}

	if c.FileNameFormatter() == nil {
		c.SetFileNameFormatter(storage.DefaultFileNameFormatter)
	}

	c.checkDeleteInterval = def.SetDefault(c.checkDeleteInterval, DefaultCheckDeleteInterval)
	if c.checkDeleteInterval < MinCheckDeleteInterval {
		c.checkDeleteInterval = MinCheckDeleteInterval
	}

	c.writeBufferSize = def.SetDefault(c.writeBufferSize, DefaultWriteBufferSize)
	if c.writeBufferSize < MinWriteBufferSize {
		c.writeBufferSize = MinWriteBufferSize
	}

	if c.writeBufferPoolProvider == nil {
		c.writeBufferPoolProvider = pool.StackPoolProvider[*Buffer](DefaultWriteBufferCount)
	}

	c.writeBufferPool = c.writeBufferPoolProvider(func(p pool.Pool[*Buffer]) *Buffer {
		return NewBuffer(c.writeBufferSize)
	})

	if c.syncReqs == nil {
		c.syncReqs = container.NewQueue[syncRequest](DefaultMaxSyncReqs)
	}

	if c.deleteFiles == nil {
		c.deleteFiles = container.NewQueue[*meta.File](DefaultMaxDeletingFiles)
	}

	if !c.CompareAndSwapLogger(nil, config.DefaultLogger().Named(c.DisplayName())) {
		c.SetLogger(c.Logger().Named(c.DisplayName()))
	}

	if metaManagerConfigOpt.provider != nil {
		c.metaManager = metaManagerConfigOpt.build(c)
	} else {
		c.metaManager = meta.NewDBMetaManager(c)
	}

	c.runner = lifecycle.NewWithInterruptedRun(c.start, c.run, lifecycle.WithSelf(c))
	c.Lifecycle = c.runner
	return c
}

type Buffer struct {
	raw []byte
	cur uint
}

func NewBuffer(size uint) *Buffer {
	return &Buffer{raw: make([]byte, size)}
}

func (b *Buffer) Reset() {
	b.cur = 0
}

func (b *Buffer) Bytes() []byte {
	return b.raw[:b.cur]
}

func (b *Buffer) Remain() uint {
	return uint(len(b.raw)) - b.cur
}

func (b *Buffer) Write(p []byte) (n int, err error) {
	n = copy(b.raw[b.cur:], p)
	b.cur += uint(n)
	return
}

type syncRequest interface {
	synchronizing() bool
	setSynchronizing()
}

type abstractSyncRequest struct {
	_synchronizing bool
}

func (r *abstractSyncRequest) synchronizing() bool {
	return r._synchronizing
}

func (r *abstractSyncRequest) setSynchronizing() {
	r._synchronizing = true
}

type blockRequest struct {
	buffer  *Buffer
	index   storage.Index
	indexes []storage.Index
	abstractSyncRequest
}

func (b *blockRequest) addIndex(index storage.Index) {
	if b.index == nil {
		b.index = index
	} else {
		b.indexes = append(b.indexes, index)
	}
}

func (b *blockRequest) rangeIndex(f func(index storage.Index) bool) {
	if b.index != nil {
		if !f(b.index) {
			return
		}
		for _, index := range b.indexes {
			if !f(index) {
				return
			}
		}
	}
}

type indexRequest struct {
	index storage.Index
	abstractSyncRequest
}

type closeFileRequest struct {
	abstractSyncRequest
}

func (c *Channel) Name() string {
	return c.name
}

func (c *Channel) DisplayName() string {
	return fmt.Sprintf(fmt.Sprintf("文件存储通道(%s)", c.name))
}

func (c *Channel) Cover() time.Duration {
	return time.Duration(c.cover.Load())
}

func (c *Channel) SetCover(cover time.Duration) storage.Channel {
	if cover < time.Minute {
		cover = time.Minute
	}
	c.cover.Store(int64(cover))
	return c
}

func (c *Channel) WriteBufferSize() uint {
	return c.writeBufferSize
}

func (c *Channel) FileNameFormatter() storage.FileNameFormatter {
	if fileNameFormatter := c.fileNameFormatter.Load(); fileNameFormatter != nil {
		return *fileNameFormatter
	}
	return nil
}

func (c *Channel) SetFileNameFormatter(formatter storage.FileNameFormatter) storage.FileChannel {
	c.fileNameFormatter.Store(&formatter)
	return c
}

func (c *Channel) Directory() string {
	if dp := c.directory.Load(); dp != nil {
		return *dp
	}
	return ""
}

func (c *Channel) SetDirectory(directory string) storage.FileChannel {
	c.directory.Store(&directory)
	return c
}

func (c *Channel) MetaManager() meta.Manager {
	return c.metaManager
}

func (c *Channel) start(_ lifecycle.Lifecycle, interrupter chan struct{}) (err error) {
	metaWaiter := c.metaManager.StartedWaiter()
	c.metaManager.Background()
loop:
	for {
		select {
		case err := <-metaWaiter:
			if err != nil {
				return err
			}
			break loop
		case <-interrupter:
			c.metaManager.Close(nil)
		}
	}

	c.syncTaskExecutor.Start()
	c.deleteTaskExecutor.Start()
	return nil
}

func (c *Channel) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	ticker := time.NewTicker(c.checkDeleteInterval)
	defer func() {
		ticker.Stop()
		c.readSessions.Range(func(session *ReadSession, _ struct{}) bool {
			session.Shutdown()
			return true
		})

		c.closeFile()
		c.syncTaskExecutor.Close(nil)
		c.deleteTaskExecutor.Close(nil)
		<-c.syncTaskExecutor.ClosedWaiter()
		<-c.deleteTaskExecutor.ClosedWaiter()
		c.metaManager.Shutdown()
	}()

	var curFile *meta.File
	deleteCurFile := func() {
		if lock.LockGet(&c.deleteFilesLock, func() bool { return c.deleteFiles.Push(curFile) }) {
			curFile = nil
			c.Logger().Debug("最早的文件已没有索引指向，删除此文件")
			c.deleteTaskExecutor.Try(taskPkg.Interrupted(c.doDelete))
		} else {
			c.Logger().Debug("删除文件的任务已满，忽略此次提交的任务")
		}
	}
	for {
		select {
		case now := <-ticker.C:
			// 检查最早的索引是否过期
			first, _ := c.metaManager.FirstIndex()
			if first != nil {
				offsetNow := c.metaManager.Offset(now.UnixMilli())
				if offsetNow-first.End() > c.Cover().Milliseconds() {
					c.Logger().Debug("最早的索引已过期，删除此索引",
						log.Object("索引", first),
						log.Int64("当前时间", offsetNow),
						log.Int64("最早索引的结束时间", first.End()),
						log.Int64("覆盖周期", c.Cover().Milliseconds()),
					)
					// 删除过期的索引
					c.metaManager.DeleteIndex()
					// 判断索引对应的文件是否需要删除。有些索引没有指向文件(索引的文件序列号为0，如流丢
					// 失索引)，这些索引不需要判断
					if first.FileSeq() != 0 {
						if curFile != nil && first.FileSeq() > curFile.Seq {
							// 当前索引对应的文件序号高于上一个索引对应的文件，删除上一个索引对应的文件
							deleteCurFile()
						}
						if file := c.metaManager.GetFile(first.FileSeq()); file != nil {
							curFile = file
							if next, loaded := c.metaManager.FirstIndex(); loaded {
								if next == nil || (next.FileSeq() != 0 && next.FileSeq() > first.FileSeq()) {
									// 下一个索引不存在或是对应的文件序号高于当前索引对应的文件，删除
									// 当前索引对应的文件
									deleteCurFile()
								}
							}
						}
					}
					continue
				}
			} else if curFile != nil {
				deleteCurFile()
			}
		case <-interrupter:
			return nil
		}
	}
}

func (c *Channel) MakeIndexes(cap int) storage.Indexes {
	return c.metaManager.MakeIndexes(cap)
}

func (c *Channel) FindIndexes(start, end int64, limit int, indexes storage.Indexes) (storage.Indexes, error) {
	return lock.RLockGetDouble(c.runner, func() (storage.Indexes, error) {
		if !c.runner.Running() {
			return nil, ChannelNotAvailableError
		}
		return c.metaManager.FindIndexes(start, end, limit, indexes)
	})
}

func (c *Channel) NewReadSession() (storage.ReadSession, error) {
	return lock.RLockGetDouble(c.runner, func() (storage.ReadSession, error) {
		if !c.runner.Running() {
			return nil, ChannelNotAvailableError
		}

		s := newReadSession(c)
		s.OnClosed(func(_ lifecycle.Lifecycle, err error) {
			c.readSessions.Delete(s)
		})
		c.readSessions.Store(s, struct{}{})
		s.Start()
		return s, nil
	})
}

func (c *Channel) openCtx(file *meta.File) error {
	res := context.OpenFile(&context.OpenRequest{
		File:        file.Path,
		Flag:        os.O_WRONLY | os.O_CREATE | os.O_SYNC,
		FileContext: file,
	})
	if res.Err != nil {
		return c.Logger().ErrorWith("打开文件失败", res.Err, log.String("文件路径", file.Path), log.Duration("花费时间", res.Elapse()))
	}
	c.fileCtx = res.Context
	c.Logger().Info("打开文件成功", log.String("文件路径", file.Path), log.Duration("花费时间", res.Elapse()))
	return nil
}

func (c *Channel) closeCtx() error {
	res := c.fileCtx.Close(&context.CloseRequest{})
	c.fileCtx = nil
	if res.Err != nil {
		return c.Logger().ErrorWith("关闭文件失败", res.Err, log.String("文件路径", res.File()), log.Duration("花费时间", res.CloseElapse))
	}
	c.Logger().Info("关闭文件成功", log.String("文件路径", res.File()), log.Duration("花费时间", res.CloseElapse))
	return nil
}

func (c *Channel) doSync() {
	reqs := lock.LockGet(&c.syncReqsLock, c.syncReqs.PopAll)
	for _, req := range reqs {
		switch request := req.(type) {
		case *blockRequest:
			var file *meta.File
			var seek *context.Seek
			// 如果文件未打开则先打开文件
			if c.fileCtx == nil {
				if c.switchFile {
					// 需要切换文件
					c.switchFile = false
				} else {
					// 第一次同步时，未打开任何文件，需检查是否需要使用最后一个文件
					if file = c.metaManager.LastFile(); file != nil {
						if file.Size < uint64(c.fileSize) && file.Duration() < c.fileDuration {
							seek = &context.Seek{Offset: int64(file.Size)}
						} else {
							file.State = flag.SwapFlagMask(file.State, meta.FileStateWriting, meta.FileStateWrote)
							c.metaManager.UpdateFile(file)
							file = nil
						}
					}
				}

				var addFile bool
				if file == nil {
					addFile = true
					file = c.metaManager.NewFile(time.Now().UnixMilli(), request.index.StorageType())
				}

				if err := c.openCtx(file); err != nil {
					request.rangeIndex(func(index storage.Index) bool {
						c.metaManager.AddIndex(index.SetState(meta.IndexStateBlockIOError))
						return true
					})
					c.writeBufferPool.Put(request.buffer)
					continue
				}
				if addFile {
					if err := c.metaManager.AddFile(file); err != nil {
						request.rangeIndex(func(index storage.Index) bool {
							c.metaManager.AddIndex(index.SetState(meta.IndexStateBlockDropped))
							return true
						})
						c.writeBufferPool.Put(request.buffer)
						continue
					}
				}
			} else {
				file = c.fileCtx.FileContext().(*meta.File)
			}

			// 写入数据块
			var offset uint64
			if seek != nil {
				offset = uint64(seek.Offset)
			} else {
				offset = uint64(c.fileCtx.Offset())
			}
			data := request.buffer.Bytes()
			res := c.fileCtx.Write(&context.WriteRequest{Data: data, Seek: seek})
			size := uint64(res.Size)
			if res.Err != nil {
				c.Logger().ErrorWith("写文件失败", res.Err,
					log.String("文件路经", res.File()),
					log.Int("数据大小", len(data)),
					log.Uint64("写入大小", size),
					log.Uint64("写入位置", offset),
					log.Duration("花费时间", res.Elapse()),
				)
			} else {
				msg := "写入文件成功"
				var logFn = c.Logger().Debug
				if elapse := res.Elapse(); elapse > time.Second {
					msg = "写入文件时间过长"
					logFn = c.Logger().Warn
				}
				logFn(msg,
					log.String("文件路经", res.File()),
					log.Int("数据大小", len(data)),
					log.Uint64("写入大小", size),
					log.Uint64("写入位置", offset),
					log.Duration("花费时间", res.Elapse()),
				)
			}
			c.writeBufferPool.Put(request.buffer)

			// 写入数据块索引
			var start, end int64
			request.rangeIndex(func(index storage.Index) bool {
				index.SetFileSeq(file.Seq).SetFileOffset(offset)
				if index.Size() > size {
					index.SetSize(size)
				}
				if res.Err != nil {
					index.SetState(meta.IndexStateBlockIOError)
				}
				offset += index.Size()
				size -= index.Size()
				c.metaManager.AddIndex(index)
				if start == 0 {
					start = index.Start()
				}
				end = index.End()
				return true
			})

			// 修改文件信息，并判断是否需要切换文件
			if file.StartTime == 0 {
				file.StartTime = start
			}
			file.EndTime = end
			file.Size = uint64(res.Context.Size())
			if file.Size >= uint64(c.fileSize) || file.Duration() >= c.fileDuration {
				file.State = flag.SwapFlagMask(file.State, meta.FileStateWriting, meta.FileStateWrote)
				c.closeCtx()
				c.metaManager.UpdateFile(file)
				c.switchFile = true
			}

		case *indexRequest:
			c.metaManager.AddIndex(request.index)

		case *closeFileRequest:
			if c.fileCtx != nil {
				file := c.fileCtx.FileContext().(*meta.File)
				c.closeCtx()
				c.metaManager.UpdateFile(file)
			}
		}
	}
}

func (c *Channel) doDelete(interrupter chan struct{}) (interrupted bool) {
	deleteFiles := lock.LockGet(&c.deleteFilesLock, c.deleteFiles.PopAll)
	for _, file := range deleteFiles {
		if err := retry.MakeRetry(retry.Retry{
			Do: func() error {
				res := context.Remove(&context.RemoveRequest{File: file.Path})
				if res.Err != nil {
					return c.Logger().ErrorWith("删除文件失败", res.Err, log.String("文件路径", res.Request.File), log.Duration("花费时间", res.DeleteElapse))
				}
				c.Logger().Info("删除文件成功", log.String("文件路径", res.Request.File), log.Duration("花费时间", res.DeleteElapse))
				c.metaManager.DeleteFile(file)
				return nil
			},
			MaxRetry:    -1,
			Interrupter: interrupter,
		}).Todo(); err == retry.InterruptedError {
			return true
		}
	}
	return false
}

func (c *Channel) sync() error {
	if c.syncReq != nil {
		if err := lock.LockGet(&c.syncReqsLock, func() error {
			c.syncReq.setSynchronizing()
			if !c.syncReqs.Push(c.syncReq) {
				return errors.New("busy")
			}
			c.syncReq = nil
			return nil
		}); err != nil {
			return err
		}
		c.syncTaskExecutor.Try(taskPkg.Func(c.doSync))
	}
	return nil
}

func (c *Channel) writeBlock(writer io.WriterTo, size uint, start, end time.Time, storageType uint32, sync bool) error {
	if size > c.writeBufferSize {
		// 写入数据超过缓冲区大小，丢弃此数据块
		return c.dropBlock(size, start, end, storageType)
	}

	doWriteBlock := func(blockReq *blockRequest) error {
		if blockReq != nil {
			if size > blockReq.buffer.Remain() {
				if err := c.sync(); err != nil {
					return err
				}
				blockReq = nil
			}
		}
		if blockReq == nil {
			blockReq = &blockRequest{
				buffer: c.writeBufferPool.Get(),
			}
			if blockReq.buffer == nil {
				return c.dropBlock(size, start, end, storageType)
			}
			blockReq.buffer.Reset()
			c.syncReq = blockReq
		}
		n, _ := writer.WriteTo(blockReq.buffer)
		blockReq.addIndex(c.metaManager.NewIndex(
			start.UnixMilli(),
			end.UnixMilli(),
			uint64(n),
			storageType,
			meta.IndexStateBlockData,
		))
		if sync {
			return c.sync()
		}
		return nil
	}

	if c.syncReq != nil {
		blockReq, is := c.syncReq.(*blockRequest)
		if !is {
			if err := c.sync(); err != nil {
				return err
			}
		}
		return doWriteBlock(blockReq)
	} else {
		return doWriteBlock(nil)
	}
}

func (c *Channel) dropBlock(size uint, start, end time.Time, storageType uint32) error {
	c.Logger().Warn("数据块被丢弃", log.Uint("数据块大小", size), log.Time("起始时间", start), log.Time("结束时间", end))
	if err := c.sync(); err != nil {
		return err
	}
	c.syncReq = &indexRequest{
		index: c.metaManager.NewIndex(
			start.UnixMilli(),
			end.UnixMilli(),
			uint64(size),
			storageType,
			meta.IndexStateBlockDropped,
		),
	}
	return c.sync()
}

func (c *Channel) streamLost(start, end time.Time) error {
	if err := c.sync(); err != nil {
		return err
	}
	c.syncReq = &indexRequest{
		index: c.metaManager.NewIndex(
			start.UnixMilli(),
			end.UnixMilli(),
			0, 0,
			meta.IndexStateBlockNotData|meta.IndexStateStreamLost,
		),
	}
	return c.sync()
}

func (c *Channel) closeFile() error {
	if err := c.sync(); err != nil {
		return err
	}
	c.syncReq = &closeFileRequest{}
	return c.sync()
}

type writer []byte

func (w writer) WriteTo(_w io.Writer) (n int64, err error) {
	in, err := _w.Write(w)
	return int64(in), err
}

func Writer(data []byte) io.WriterTo {
	return writer(data)
}

func (c *Channel) Write(writer io.WriterTo, size uint, start, end time.Time, storageType uint32, sync bool) error {
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return lifecycle.NewStateNotRunningError(c.DisplayName())
		}
		return lock.LockGet(&c.writeLock, func() error {
			return c.writeBlock(writer, size, start, end, storageType, sync)
		})
	})
}

func (c *Channel) WriteStreamLost(start, end time.Time) error {
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return lifecycle.NewStateNotRunningError(c.DisplayName())
		}
		return lock.LockGet(&c.writeLock, func() error {
			return c.streamLost(start, end)
		})
	})
}

func (c *Channel) Sync() error {
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return lifecycle.NewStateNotRunningError(c.DisplayName())
		}
		return lock.LockGet(&c.writeLock, func() error {
			return c.sync()
		})
	})
}

func (c *Channel) CloseFile() error {
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return lifecycle.NewStateNotRunningError(c.DisplayName())
		}
		return lock.LockGet(&c.writeLock, func() error {
			return c.closeFile()
		})
	})
}
