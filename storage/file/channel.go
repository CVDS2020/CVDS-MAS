package file

import (
	"fmt"
	"gitee.com/sy_183/common/def"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/flag"
	"gitee.com/sy_183/common/lifecycle"
	taskPkg "gitee.com/sy_183/common/lifecycle/task"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/storage"
	"gitee.com/sy_183/cvds-mas/storage/file/context"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var ChannelNotAvailableError = errors.New("存储通道不可用")

type FileNameFormatter func(c *Channel, seq uint64, createTime time.Time, mediaType uint32, storageType uint32) string

type Channel struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	name      string
	directory atomic.Pointer[string]
	cover     atomic.Int64

	blockSize         unit.Size
	keyFrameBlockSize unit.Size
	maxBlockSize      unit.Size

	blockDuration    time.Duration
	maxBlockDuration time.Duration

	fileSize          unit.Size
	fileDuration      time.Duration
	fileNameFormatter atomic.Value

	maxDeletingFiles    int
	checkDeleteInterval time.Duration

	writeBuffer []byte

	block                *Block
	writingBlock         atomic.Pointer[Block]
	droppedBlock         *DroppedBlock
	lastBlockMediaType   uint32
	lastBlockStorageType uint32

	ctx       *context.Context
	newCtx    *context.Context
	switchCtx atomic.Pointer[context.Context]

	fileTaskList       *taskPkg.TaskExecutor
	fileDeleteTaskList *taskPkg.TaskExecutor
	writer             *taskPkg.TaskExecutor
	deleter            lifecycle.Lifecycle
	metaManager        *dbMetaManager

	readSessions sync.Map

	writeLock sync.Mutex

	log.AtomicLogger
}

func NewChannel(name string, options ...storage.ChannelOption) *Channel {
	c := &Channel{
		name:         name,
		fileTaskList: taskPkg.NewTaskExecutor(1),
		writer:       taskPkg.NewTaskExecutor(1),
	}
	c.deleter = lifecycle.NewWithInterruptedRun(nil, c.deleterRun)
	var dbMetaManagerOpts dbMetaManagerOptions
	for _, option := range options {
		switch opt := option.Apply(c).(type) {
		case dbMetaManagerOptions:
			dbMetaManagerOpts = opt
		}
	}

	c.cover.CompareAndSwap(0, int64(time.Hour*24*7))
	if cover := c.cover.Load(); time.Duration(cover) < time.Minute {
		c.cover.Store(int64(time.Minute))
	}

	// 数据块默认大小1M
	c.blockSize = def.SetDefault(c.blockSize, unit.MeBiByte)
	if c.blockSize < 64*unit.KiBiByte {
		// 数据块最小64K
		c.blockSize = 64 * unit.KiBiByte
	}
	if c.keyFrameBlockSize < c.blockSize {
		c.keyFrameBlockSize = c.blockSize
	}
	if c.maxBlockSize < c.keyFrameBlockSize {
		c.maxBlockSize = c.keyFrameBlockSize
	}
	// 数据块默认持续时间为5s
	c.blockDuration = def.SetDefault(c.blockDuration, time.Second*5)
	if c.blockDuration < time.Millisecond*100 {
		// 数据块最小持续时间为100ms
		c.blockDuration = 100 * time.Millisecond
	}
	if c.maxBlockDuration < c.blockDuration {
		c.maxBlockDuration = c.blockDuration
	}

	// 文件大小默认512M
	c.fileSize = def.SetDefault(c.fileSize, 512*unit.MeBiByte)
	if c.fileSize < unit.MeBiByte {
		// 文件大小最小为1M
		c.fileSize = unit.MeBiByte
	}
	// 文件默认持续时间为30分钟
	c.fileDuration = def.SetDefault(c.fileDuration, 30*time.Minute)
	if c.fileDuration < time.Minute {
		// 文件最小持续时间为1分钟
		c.fileDuration = time.Minute
	}

	// 默认最大同时删除的文件为16个
	c.maxDeletingFiles = def.SetDefault(c.maxDeletingFiles, 16)
	if c.maxDeletingFiles < 1 {
		// 最大同时删除的文件个数的最小值为1个
		c.maxDeletingFiles = 1
	}
	// 默认检查过期索引是否要删除的间隔时间为1s
	c.checkDeleteInterval = def.SetDefault(c.checkDeleteInterval, time.Second)
	if c.checkDeleteInterval < 50*time.Millisecond {
		// 最小检查过期索引是否要删除的间隔时间为50ms
		c.checkDeleteInterval = 50 * time.Millisecond
	}

	c.writeBuffer = make([]byte, c.maxBlockSize)
	c.fileDeleteTaskList = taskPkg.NewTaskExecutor(c.maxDeletingFiles)

	if !c.CompareAndSwapLogger(nil, Logger().Named(c.DisplayName())) {
		c.SetLogger(c.Logger().Named(c.DisplayName()))
	}
	c.fileNameFormatter.CompareAndSwap(nil, FileNameFormatter(func(c *Channel, seq uint64, createTime time.Time, mediaType uint32, storageType uint32) string {
		var suffix string
		if typ := storage.GetStorageType(storageType); typ != nil {
			suffix = "." + typ.Suffix
		}
		return fmt.Sprintf("%s-%s-%d%s", c.Name(), createTime.Format("20060102150405"), seq, suffix)
	}))

	if dbMetaManagerOpts != nil {
		c.metaManager = NewDBMetaManager(c, dbMetaManagerOpts...)
	} else {
		c.metaManager = NewDBMetaManager(c)
	}

	c.runner = lifecycle.NewWithInterruptedRun(c.start, c.run)
	c.Lifecycle = c.runner
	return c
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

func (c *Channel) FileNameFormatter() FileNameFormatter {
	return c.fileNameFormatter.Load().(FileNameFormatter)
}

func (c *Channel) SetFileNameFormatter(formatter FileNameFormatter) *Channel {
	c.fileNameFormatter.Store(formatter)
	return c
}

func (c *Channel) Directory() string {
	if dp := c.directory.Load(); dp != nil {
		return *dp
	}
	return ""
}

func (c *Channel) SetDirectory(directory string) *Channel {
	c.directory.Store(&directory)
	return c
}

func (c *Channel) BlockSize() unit.Size {
	return c.blockSize
}

func (c *Channel) KeyFrameBlockSize() unit.Size {
	return c.keyFrameBlockSize
}

func (c *Channel) MaxBlockSize() unit.Size {
	return c.maxBlockSize
}

func (c *Channel) FileSize() unit.Size {
	return c.fileSize
}

func (c *Channel) openFile(file *File) error {
	res := context.OpenFile(&context.OpenRequest{
		File:        file.Path,
		Flag:        os.O_WRONLY | os.O_CREATE | os.O_SYNC,
		FileContext: file,
	})
	if res.Err != nil {
		c.switchCtx.Store(nil)
		c.Logger().ErrorWith("打开文件失败", res.Err, log.String("文件路径", res.File()), log.Duration("花费时间", res.Elapse()))
		return res.Err
	}
	c.Logger().Info("打开文件成功", log.String("文件路径", res.File()), log.Duration("花费时间", res.Elapse()))
	if err := c.metaManager.AddFile(file); err != nil {
		c.newCtx = res.Context
		c.switchCtx.Store(nil)
		return err
	} else {
		c.switchCtx.Store(res.Context)
	}
	return nil
}

func (c *Channel) closeCtx(ctx *context.Context) error {
	res := ctx.Close(&context.CloseRequest{})
	if res.Err != nil {
		c.Logger().ErrorWith("关闭文件失败", res.Err, log.String("文件路径", res.File()), log.Duration("花费时间", res.CloseElapse))
		return res.Err
	}
	c.Logger().Info("关闭文件成功", log.String("文件路径", res.File()), log.Duration("花费时间", res.CloseElapse))
	return nil
}

func (c *Channel) closeAllCtx() {
	if c.ctx != nil {
		file := c.ctx.FileContext().(*File)
		file.State = flag.SwapFlagMask(file.State, FileStateWriting, FileStateWrote)
		c.closeCtx(c.ctx)
		c.metaManager.UpdateFile(file)
	}
	if switchCtx := c.switchCtx.Load(); switchCtx != nil && switchCtx != c.ctx {
		file := switchCtx.FileContext().(*File)
		file.State = flag.SwapFlagMask(file.State, FileStateWriting, FileStateWrote)
		c.closeCtx(switchCtx)
		c.metaManager.UpdateFile(file)
	}
	if c.newCtx != nil {
		c.closeCtx(c.newCtx)
	}
	c.ctx = nil
	c.newCtx = nil
	c.switchCtx.Store(nil)
}

func (c *Channel) write(switchFile bool) {
	block := c.writingBlock.Load()
	if block == nil {
		return
	}

	if block.IndexState&(0b11<<1) == IndexStateBlockNotData {
		index := c.metaManager.BuildIndex(&Index{
			Start: block.Start.UnixMilli(),
			End:   block.End.UnixMilli(),
			State: block.IndexState,
		})
		c.metaManager.AddIndex(index)
		block.Free()
		return
	}

	// switchCtx: 文件切换上下文。文件切换分为两个部分，第一部分为新文件的打开，第二部分为
	// 旧文件的关闭。在文件切换期间，文件切换上下文不为 nil，同时文件切换上下文为原子变量，
	// 从而保证了串行的切换文件上下文
	switchCtx := c.switchCtx.Load()
	if switchCtx != nil || switchFile || c.ctx == nil {
		// switchCtx != nil 代表正在切换文件
		// switchFile 代表要求切换文件
		// c.ctx == nil 代表需要打开文件
		if switchFile || c.ctx == nil {
			if switchCtx != nil && c.ctx == switchCtx {
				// 正在打开文件，等待文件打开完成，文件打开完成后，获取 switchCtx，此时
				// switchCtx 为空则说明文件打开失败
				c.fileTaskList.Wait()
				switchCtx = c.switchCtx.Load()
				if switchCtx != nil {
					file := switchCtx.FileContext().(*File)
					if file.MediaType != block.MediaType || file.StorageType != block.StorageType {
						file.MediaType = block.MediaType
						file.StorageType = block.StorageType
						c.metaManager.UpdateFile(file)
					}
				}
			} else if switchCtx == nil {
				// 不在切换文件，则主动切换文件
				if c.newCtx != nil {
					// 文件打开成功，但是文件信息没有写入成功，重新写入文件信息
					newFile := c.newCtx.FileContext().(*File)
					newFile.MediaType = block.MediaType
					newFile.StorageType = block.StorageType
					if err := c.metaManager.AddFile(newFile); err == nil {
						// 文件信息写入成功
						switchCtx = c.newCtx
						c.switchCtx.Store(switchCtx)
						c.newCtx = nil
					}
				} else {
					// 以同步方式打开文件，文件打开完成后，获取 switchCtx，此时
					// switchCtx 为空则说明文件打开失败
					newFile := c.metaManager.NewFile(time.Now().UnixMilli(), block.MediaType, block.StorageType)
					c.openFile(newFile)
					switchCtx = c.switchCtx.Load()
				}
			}
		}
		if c.ctx != switchCtx {
			// 文件切换上下文有变化，此时需要关闭旧文件(如果旧文件存在)
			if oldCtx := c.ctx; oldCtx != nil {
				c.fileTaskList.Async(taskPkg.Func(func() {
					oldFile := oldCtx.FileContext().(*File)
					oldFile.State = flag.SwapFlagMask(oldFile.State, FileStateWriting, FileStateWrote)
					c.closeCtx(oldCtx)
					c.metaManager.UpdateFile(oldFile)
					c.switchCtx.Store(nil)
				}))
			} else {
				c.switchCtx.Store(nil)
			}
			c.ctx = switchCtx
		}
	}

	//size := block.Size()
	if c.ctx == nil {
		// 文件打开失败，作为IO错误写入索引中
		index := c.metaManager.BuildIndex(&Index{
			Start: block.Start.UnixMilli(),
			End:   block.End.UnixMilli(),
			State: block.IndexState,
		})
		index.State |= IndexStateBlockIOError
		c.metaManager.AddIndex(index)
		block.Free()
		return
	}
	file := c.ctx.FileContext().(*File)

	n, _ := block.Read(c.writeBuffer)
	data := c.writeBuffer[:n]

	//data := c.writeBufferPool.Alloc(size)
	//block.Read(data.Data)
	// 获取写之前的文件指针偏移
	offset := c.ctx.Offset()
	res := c.ctx.Write(&context.WriteRequest{Data: data})
	if res.Err != nil {
		c.Logger().ErrorWith("写文件失败", res.Err,
			log.String("文件路经", res.File()),
			log.Int("数据大小", len(data)),
			log.Int64("写入大小", res.Size),
			log.Int64("写入位置", res.Offset),
			log.Duration("花费时间", res.Elapse()),
		)
	} else {
		c.Logger().Debug("写入文件成功",
			log.String("文件路经", res.File()),
			log.Int("数据大小", len(data)),
			log.Int64("写入大小", res.Size),
			log.Int64("写入位置", res.Offset),
			log.Duration("花费时间", res.Elapse()),
		)
	}

	// 创建并插入索引
	index := c.metaManager.BuildIndex(&Index{
		Start:      block.Start.UnixMilli(),
		End:        block.End.UnixMilli(),
		FileSeq:    file.Seq,
		FileOffset: uint64(offset),
		Size:       uint64(res.Size),
		State:      block.IndexState,
	})
	if res.Err != nil {
		index.State |= IndexStateBlockIOError
	}
	c.metaManager.AddIndex(index)
	block.Free()

	if file.StartTime == 0 {
		file.StartTime = index.Start
	}
	file.EndTime = index.End
	file.Size = uint64(res.Context.Size())
	if file.Size >= uint64(c.fileSize) || file.Duration() >= c.fileDuration {
		if c.switchCtx.CompareAndSwap(nil, c.ctx) {
			if c.newCtx != nil {
				newFile := c.newCtx.FileContext().(*File)
				newFile.MediaType = file.MediaType
				newFile.StorageType = file.StorageType
				if err := c.metaManager.AddFile(newFile); err != nil {
					c.switchCtx.Store(nil)
				} else {
					c.switchCtx.Store(c.newCtx)
					c.newCtx = nil
				}
			} else {
				newFile := c.metaManager.NewFile(time.Now().UnixMilli(), file.MediaType, file.StorageType)
				c.fileTaskList.Async(taskPkg.Func(func() { c.openFile(newFile) }))
			}
		}
	}
}

func (c *Channel) deleterRun(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	ticker := time.NewTicker(c.checkDeleteInterval)
	defer ticker.Stop()

	for {
		select {
		case now := <-ticker.C:
			// 检查最早的索引是否过期
			first := c.metaManager.FirstIndex()
			//last := c.metaManager.LastIndex()
			if first != nil {
				if c.metaManager.Offset(now.UnixMilli())-first.End > c.Cover().Milliseconds() {
					c.Logger().Debug("最早的索引已过期，删除此索引", log.Object("索引", first), log.Int64("now", c.metaManager.Offset(now.UnixMilli())), log.Int64("first", first.End), log.Int64("cover", c.Cover().Milliseconds()))
					// 删除过期的索引
					c.metaManager.DeleteIndex()
					if first.FileSeq != 0 {
						if firstFile := c.metaManager.FirstFile(); firstFile.Seq < first.FileSeq {
							c.Logger().Debug("最早的文件已没有索引指向，删除此文件")
							if ok, _ := c.fileDeleteTaskList.Try(taskPkg.Func(func() {
								res := context.Remove(&context.RemoveRequest{File: firstFile.Path})
								if res.Err != nil {
									c.Logger().ErrorWith("删除文件失败", res.Err, log.String("文件路径", res.Request.File), log.Duration("花费时间", res.DeleteElapse))
									return
								}
								c.Logger().Info("删除文件成功", log.String("文件路径", res.Request.File), log.Duration("花费时间", res.DeleteElapse))
							})); ok {
								c.metaManager.DeleteFile(firstFile)
							} else {
								c.Logger().Warn("删除文件的任务已满，忽略此次提交的任务")
							}
						}
					}
					continue
				}
			}
		case <-interrupter:
			return nil
		}
	}
}

func (c *Channel) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	go c.metaManager.Start()

	select {
	case err := <-c.metaManager.StartedWaiter():
		if err != nil {
			return err
		}
	case <-interrupter:
		return lifecycle.NewInterruptedError(c.DisplayName(), "启动")
	}

	c.fileTaskList.Start()
	c.fileDeleteTaskList.Start()
	c.deleter.Start()
	c.writer.Start()

	return nil
}

func (c *Channel) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	<-interrupter

	c.readSessions.Range(func(key, _ any) bool {
		key.(*ReadSession).Shutdown()
		return true
	})

	// writer 依赖 metaManager, fileTaskList
	c.sync()
	c.writer.Shutdown()
	c.closeAllCtx()

	// deleter 依赖 metaManager, fileDeleteTaskList
	c.deleter.Shutdown()
	// fileDeleteTaskList 不依赖其他组件
	c.fileDeleteTaskList.Close(nil)
	// fileTaskList 依赖 metaManager
	c.fileTaskList.Shutdown()
	// metaManager 不依赖其他组件
	c.metaManager.Close(nil)
	<-c.fileDeleteTaskList.ClosedWaiter()
	<-c.metaManager.ClosedWaiter()

	return nil
}

func (c *Channel) MakeIndexes(cap int) storage.Indexes {
	return make(Indexes, 0, cap)
}

func (c *Channel) FindIndexes(start, end int64, limit int, indexes storage.Indexes) (storage.Indexes, error) {
	return lock.RLockGetDouble(c.runner, func() (storage.Indexes, error) {
		if !c.runner.Running() {
			return nil, ChannelNotAvailableError
		}
		return c.metaManager.FindIndexes(start, end, limit, indexes.(Indexes))
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

func (c *Channel) needSwitchFile(block *Block) bool {
	return block != nil && block.Len() > 0 &&
		(block.MediaType != c.lastBlockMediaType || block.StorageType != c.lastBlockStorageType)
}

func (c *Channel) asyncWrite(blocks ...*Block) bool {
	block := c.block
	if block == nil {
		if len(blocks) == 0 {
			return false
		}
		block = blocks[0]
		blocks = blocks[1:]
	}
	if c.writingBlock.CompareAndSwap(nil, block) {
		// 上一次的写任务已完成，开始异步写入此数据块，在写入数据块之前，检查之前是否有丢
		// 弃的帧，如果有，则将丢帧信息添加到索引中
		if c.droppedBlock != nil {
			c.metaManager.AddIndex(c.metaManager.BuildIndex(&Index{
				Start: c.droppedBlock.Start.UnixMilli(),
				End:   c.droppedBlock.Start.UnixMilli(),
				Size:  uint64(c.droppedBlock.Size()),
				State: IndexStateBlockDropped,
			}))
			c.droppedBlock = nil
		}
		switchFile := c.needSwitchFile(block)
		if block.MediaType != 0 && block.StorageType != 0 {
			c.lastBlockMediaType = block.MediaType
			c.lastBlockStorageType = block.StorageType
		}
		switchFiles := make([]bool, len(blocks))
		for i, block := range blocks {
			switchFiles[i] = c.needSwitchFile(block)
			if block.MediaType != 0 && block.StorageType != 0 {
				c.lastBlockMediaType = block.MediaType
				c.lastBlockStorageType = block.StorageType
			}
		}
		if err := c.writer.Async(taskPkg.Func(func() {
			c.write(switchFile)
			for i, block := range blocks {
				c.writingBlock.Store(block)
				c.write(switchFiles[i])
			}
			c.writingBlock.Store(nil)
		})); err != nil {
			// 由于写任务线程已关闭，添加到写任务失败，释放 blocks 中的数据块，但不释
			// 放 c.block
			if block != c.block {
				block.Free()
			}
			for _, block := range blocks {
				block.Free()
			}
			c.writingBlock.Store(nil)
			return false
		}
		c.block = nil
		return true
	}
	return false
}

func (c *Channel) sync() {
	if c.droppedBlock == nil && c.block == nil {
		return
	}

	c.writer.Sync(taskPkg.Func(func() {
		if c.droppedBlock != nil {
			c.metaManager.AddIndex(c.metaManager.BuildIndex(&Index{
				Start: c.droppedBlock.Start.UnixMilli(),
				End:   c.droppedBlock.Start.UnixMilli(),
				Size:  uint64(c.droppedBlock.Size()),
				State: IndexStateBlockDropped,
			}))
			c.droppedBlock = nil
		}
		if c.block != nil {
			c.writingBlock.Store(c.block)
			c.write(c.needSwitchFile(c.block))
			c.writingBlock.Store(nil)
			c.block = nil
		}
	}))
}

func (c *Channel) Write(frame storage.Frame) bool {
	return lock.RLockGet(c.runner, func() bool {
		if !c.runner.Running() {
			return false
		}

		return lock.LockGet(&c.writeLock, func() bool {
			var size uint
			var duration time.Duration
			if c.block != nil {
				size = c.block.Size()
				duration = c.block.Duration()
			}
			frameDuration := frame.EndTime().Sub(frame.StartTime())

			if frame.Size() > uint(c.maxBlockSize) || frameDuration > c.maxBlockDuration {
				if c.droppedBlock == nil {
					c.droppedBlock = new(DroppedBlock)
				}
				c.droppedBlock.Append(frame)
				return false
			}

			// 切换数据块的条件一：数据块的媒体类型或存储类型发生改变
			cond1 := c.block != nil && c.block.Len() > 0 && (frame.MediaType() != c.block.MediaType || frame.StorageType() != c.block.StorageType)
			// 切换数据块的条件二：数据块的大小加上当前帧的大小或持续时间加上当前帧的持续时间后
			// 超过了最大限定值
			cond2 := size+frame.Size() > uint(c.maxBlockSize) || duration+frameDuration > c.maxBlockDuration

			if c.droppedBlock != nil || (cond1 || cond2) {
				// 如果上一帧被丢弃，此时重新尝试添加写任务，或是满足上面这两个切换数据块的条件
				// 之一，此时如果没有添加成功，则会引起当前帧丢弃
				if !c.asyncWrite() {
					if c.droppedBlock == nil {
						c.droppedBlock = new(DroppedBlock)
					}
					c.droppedBlock.Append(frame)
					return false
				}
			} else if c.block != nil && size >= uint(c.blockSize) || duration >= c.blockDuration {
				// 当前的块大小或持续时间超过了阈值
				if frame.KeyFrame() || size >= uint(c.keyFrameBlockSize) || duration >= c.blockDuration {
					// 如果此帧为关键帧，则尝试将关键帧之前的数据块添加到写任务中，如果此帧不是
					// 关键帧，但是块大小或持续时间超过了关键帧阈值，也尝试将数据块添加到写任务
					// 中。如果没有添加成功，也不会引起丢帧
					c.asyncWrite()
				}
			}

			if c.block == nil {
				c.block = new(Block)
			}
			c.block.Append(frame)

			return true
		})
	})
}

func (c *Channel) WriteStreamLost(start, end time.Time) bool {
	return lock.RLockGet(c.runner, func() bool {
		if !c.runner.Running() {
			return false
		}

		return lock.LockGet(&c.writeLock, func() bool {
			if !c.asyncWrite(&Block{
				Start:      start,
				End:        end,
				IndexState: IndexStateBlockNotData | IndexStateStreamLost,
			}) {
				if c.droppedBlock != nil {
					c.droppedBlock = new(DroppedBlock)
				}
				c.droppedBlock.Add(start, end, 0)
				return false
			}
			return true
		})
	})
}

func (c *Channel) Sync() {
	lock.RLockDo(c.runner, func() {
		if !c.runner.Running() {
			return
		}
		lock.LockDo(&c.writeLock, func() {
			c.asyncWrite()
		})
	})
}
