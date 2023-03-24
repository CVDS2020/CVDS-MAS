package meta

import (
	"fmt"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lifecycle/retry"
	taskPkg "gitee.com/sy_183/common/lifecycle/task"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/slice"
	"gitee.com/sy_183/common/utils"
	"gitee.com/sy_183/cvds-mas/config"
	dbPkg "gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/storage"
	"gorm.io/gorm"
	"os"
	"path"
	"sort"
	"sync"
	"time"
)

const (
	DBMetaManagerModule = Module + ".db-meta-manager"
	DBManagerModule     = Module + ".db"
)

const (
	DefaultDBDSN = "cvds:cvds2020@(localhost:3306)/cvdsrec"

	MinEarliestIndexesCacheSize = 16

	MinNeedCreatedIndexesCacheSize = 16

	MinMaxFiles = 16

	// 默认缓存最早的 64 个索引
	DefaultEarliestIndexesCacheSize = 64

	// 默认缓存未添加到数据库中的 1024 个索引
	DefaultNeedCreatedIndexesCacheSize = 1024

	// 默认最大 4K 个文件
	DefaultMaxFiles = 4096
)

var NotFileChannelError = errors.New("存储通道不是文件存储类型")

type taskType int

const (
	taskTypeFileCreate = taskType(iota)
	taskTypeFileUpdate
	taskTypeFileDelete
)

type fileChecker struct {
	startSeq      uint64
	endSeq        uint64
	seqErrorCount int
}

func newFileChecker(startSeq, endSeq uint64) *fileChecker {
	return &fileChecker{
		startSeq: startSeq,
		endSeq:   endSeq,
	}
}

func (c *fileChecker) Check(file *File) bool {
	if c.startSeq != 0 {
		if file.Seq <= c.startSeq {
			c.seqErrorCount++
			return false
		}
	}
	c.startSeq = file.Seq
	if c.endSeq != 0 {
		if file.Seq >= c.endSeq {
			c.seqErrorCount++
			return false
		}
	}
	return true
}

func (c *fileChecker) SeqErrorCount() int {
	return c.seqErrorCount
}

type DBMetaManager struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	// 通道
	channel storage.Channel

	dbManager *dbPkg.DBManager

	// 通道信息缓存
	channelInfo *ChannelInfo
	// 更新通道信息的任务列表
	channelUpdateTaskList *taskPkg.TaskExecutor
	// 通道信息缓存锁
	channelLock sync.Mutex

	// 最早索引缓存，用于判断是否有索引需要删除
	earliestIndexes *container.Queue[Index]
	// 被加载到最早索引缓存的索引数量，但删除索引后不会减少此数量，所以一旦此数量小于最早索引缓存的容量，
	// 则说明还有索引未被加载到缓存中
	loadableEarliestIndexes int
	// 最新的索引，用于添加新索引时检查索引的合法性
	latestIndex *Index
	// 等待被添加到数据库中的索引
	needCreatedIndexes *container.Queue[Index]
	// 等待被删除的索引数量
	needDeletedIndexes int
	// 添加索引的任务列表
	addIndexTaskList *taskPkg.TaskExecutor
	// 删除索引的任务列表
	deleteIndexTaskList *taskPkg.TaskExecutor
	// 扩充最早索引的任务列表
	growEarliestIndexesTaskList *taskPkg.TaskExecutor
	// 索引缓存锁
	indexLock sync.Mutex

	// 文件信息缓存
	enableFile       bool
	files            *container.Queue[File]
	needCreatedFiles *container.LinkedMap[uint64, *File]
	needUpdatedFiles *container.LinkedMap[uint64, *File]
	needDeletedFiles *container.LinkedMap[uint64, struct{}]
	fileTasks        *container.LinkedMap[taskType, func()]
	fileTaskExecutor *taskPkg.TaskExecutor
	// 文件信息缓存锁
	filesLock sync.Mutex

	log.AtomicLogger
}

func NewDBMetaManager(channel storage.Channel, options ...Option) *DBMetaManager {
	m := &DBMetaManager{
		channel: channel,

		channelUpdateTaskList:       taskPkg.NewTaskExecutor(1),
		addIndexTaskList:            taskPkg.NewTaskExecutor(1),
		deleteIndexTaskList:         taskPkg.NewTaskExecutor(1),
		growEarliestIndexesTaskList: taskPkg.NewTaskExecutor(1),

		needCreatedFiles: container.NewLinkedMap[uint64, *File](0),
		needUpdatedFiles: container.NewLinkedMap[uint64, *File](0),
		needDeletedFiles: container.NewLinkedMap[uint64, struct{}](0),
		fileTasks:        container.NewLinkedMap[taskType, func()](0),
		fileTaskExecutor: taskPkg.NewTaskExecutor(1),
	}
	for _, option := range options {
		option.Apply(m)
	}
	if m.dbManager == nil {
		m.dbManager = dbPkg.NewDBManager(nil, DefaultDBDSN, dbPkg.WithTablesInfo(MetaTablesInfos))
	}
	if m.earliestIndexes == nil {
		m.earliestIndexes = container.NewQueue[Index](DefaultEarliestIndexesCacheSize)
	}
	if m.needCreatedIndexes == nil {
		m.needCreatedIndexes = container.NewQueue[Index](DefaultNeedCreatedIndexesCacheSize)
	}
	if m.files == nil {
		m.files = container.NewQueue[File](DefaultMaxFiles)
	}
	logger := channel.Logger()
	if logger == nil {
		logger = config.DefaultLogger()
	}
	if !m.CompareAndSwapLogger(nil, logger.Named(m.DisplayName())) {
		m.SetLogger(m.Logger().Named(m.DisplayName()))
	}
	m.dbManager.SetLogger(m.Logger())
	m.runner = lifecycle.NewWithInterruptedRun(m.start, m.run)
	m.Lifecycle = m.runner
	return m
}

func ProvideDBMetaManager(channel storage.Channel, options ...Option) Manager {
	return NewDBMetaManager(channel, options...)
}

func (m *DBMetaManager) setEarliestIndexesCacheSize(size int) {
	if size == 0 {
		size = DefaultEarliestIndexesCacheSize
	}
	if size < MinEarliestIndexesCacheSize {
		size = MinEarliestIndexesCacheSize
	}
	m.earliestIndexes = container.NewQueue[Index](size)
}

func (m *DBMetaManager) setNeedCreatedIndexesCacheSize(size int) {
	if size == 0 {
		size = DefaultNeedCreatedIndexesCacheSize
	}
	if size < MinNeedCreatedIndexesCacheSize {
		size = MinNeedCreatedIndexesCacheSize
	}
	m.needCreatedIndexes = container.NewQueue[Index](size)
}

func (m *DBMetaManager) setMaxFiles(maxFiles int) {
	if maxFiles == 0 {
		maxFiles = DefaultMaxFiles
	}
	if maxFiles < MinMaxFiles {
		maxFiles = MinMaxFiles
	}
	m.files = container.NewQueue[File](maxFiles)
}

func (*DBMetaManager) tableName(name string) func(tx *gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		return tx.Table(name)
	}
}

func (m *DBMetaManager) indexTable() *gorm.DB {
	return m.dbManager.Table("index_" + m.channel.Name())
}

func (m *DBMetaManager) fileTable() *gorm.DB {
	return m.dbManager.Table("file_" + m.channel.Name())
}

func (m *DBMetaManager) retryDo(interrupter chan struct{}, fn func() error) bool {
	if err := retry.MakeRetry(retry.Retry{
		Do:          fn,
		MaxRetry:    -1,
		Interrupter: interrupter,
	}).Todo(); err == retry.InterruptedError {
		return false
	}
	if _, interrupted := utils.ChanTryPop(interrupter); interrupted {
		return false
	}
	return true
}

// ensureIndexTable 在初始化时如果没有索引信息表则创建
func (m *DBMetaManager) ensureIndexTable(interrupter chan struct{}) bool {
	return m.dbManager.EnsureTable(&dbPkg.TableInfo{
		Name:    "index_" + m.channel.Name(),
		Comment: fmt.Sprintf("通道(%s)索引表", m.channel.Name()),
		Model:   IndexModel,
	}, interrupter)
}

// ensureFileTable 在初始化时如果没有文件信息表则创建
func (m *DBMetaManager) ensureFileTable(interrupter chan struct{}) bool {
	return m.dbManager.EnsureTable(&dbPkg.TableInfo{
		Name:    "file_" + m.channel.Name(),
		Comment: fmt.Sprintf("通道(%s)文件信息表", m.channel.Name()),
		Model:   FileModel,
	}, interrupter)
}

// loadChannelInfo 在初始化时加载通道信息，如果没有通道信息则创建
func (m *DBMetaManager) loadChannelInfo(interrupter chan struct{}) bool {
	var channelNeedCreate bool
	return m.retryDo(interrupter, func() error {
		channelInfo := &ChannelInfo{}
	createChannel:
		// channel info not found, create it
		if channelNeedCreate {
			channelInfo.Name = m.channel.Name()
			channelInfo.Location = curLocationName()
			channelInfo.LocationOffset = curLocationOffset()
			if res := m.dbManager.Table(ChannelInfoTableName).Create(channelInfo); res.Error != nil {
				m.Logger().ErrorWith("向数据库中添加通道信息失败", res.Error)
				return res.Error
			}
			m.Logger().Info("向数据库中添加通道信息成功", log.Object("通道信息", channelInfo))
			m.channelInfo = channelInfo
			return nil
		}
		// query channel info from database
		if res := m.dbManager.Table(ChannelInfoTableName).Where("name = ?", m.channel.Name()).First(channelInfo); res.Error != nil {
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				m.Logger().Info("未找到通道信息，创建通道信息")
				channelNeedCreate = true
				goto createChannel
			}
			m.Logger().ErrorWith("从数据库中获取通道信息失败", res.Error)
			return res.Error
		}
		m.Logger().Info("从数据库中获取通道信息成功", log.Object("通道信息", channelInfo))
		m.channelInfo = channelInfo
		return nil
	})
}

// loadFiles 在初始化时从数据库中加载所有的文件信息
func (m *DBMetaManager) loadFiles(interrupter chan struct{}) bool {
	if !m.retryDo(interrupter, func() error {
		var files []File
		if res := m.fileTable().Order("seq desc").Limit(m.files.Cap()).Find(&files); res.Error != nil {
			m.Logger().ErrorWith("从数据库中读取文件信息失败", res.Error, log.Int("文件信息读取限制数量", m.files.Cap()))
			return res.Error
		}
		m.Logger().Info("从数据库中读取文件信息成功", log.Int("文件信息读取数量", len(files)))
		if len(files) > m.files.Cap() {
			files = files[:m.files.Cap()]
			m.Logger().Error("从数据库中读取的文件信息数量超过限制，丢弃多余的文件信息",
				log.Int("文件信息读取限制数量", m.files.Cap()),
				log.Int("文件信息读取数量", len(files)),
			)
		}
		checker := newFileChecker(0, 0)
		lock.LockDo(&m.filesLock, func() {
			for i := len(files) - 1; i >= 0; i-- {
				file := &files[i]
				if checker.Check(file) {
					m.files.Push(*file)
				}
			}
		})
		if seqErrorCount := checker.SeqErrorCount(); seqErrorCount > 0 {
			m.Logger().Error("发现错误的文件信息，错误原因为文件信息的序列号不大于上一个文件信息的序列号", log.Int("此错误原因的索引数量", seqErrorCount))
		}
		return nil
	}) {
		return false
	}

	var lastFile1, lastFile2 *File
	lock.LockDo(&m.filesLock, func() {
		fileCount := m.files.Len()
		if fileCount > 0 {
			lastFile1 = m.files.TailPointer().Clone()
		}
		if fileCount > 1 {
			lastFile2 = m.files.Pointer(fileCount - 2).Clone()
		}
	})
	if lastFile1 != nil {
		oldLastFile := lastFile1.Clone()
		if fileLastIndex := m.LastIndex(); fileLastIndex != nil && fileLastIndex.FileSeq() == lastFile1.Seq {
			var fileFirstIndex Index
			if !m.retryDo(interrupter, func() error {
				if res := m.indexTable().Where("fileSeq = ?", lastFile1.Seq).First(&fileFirstIndex); res.Error != nil {
					return m.Logger().ErrorWith("从数据库查找指定文件的索引失败", res.Error, log.Uint64("文件序列号", lastFile1.Seq))
				}
				m.Logger().Debug("从数据中查找指定文件的索引成功", log.Object("索引", &fileFirstIndex))
				return nil
			}) {
				return false
			}
			lastFile1.StartTime = fileFirstIndex.StartV
			lastFile1.EndTime = fileLastIndex.End()
			if lastFile1.EndTime < lastFile1.StartTime {
				return true
			}
			if lastFile2 != nil {
				if lastFile1.Seq <= lastFile2.Seq || lastFile1.StartTime < lastFile2.EndTime {
					return true
				}
			}

			if !m.retryDo(interrupter, func() error {
				fileInfo, err := os.Stat(lastFile1.Path)
				if err != nil {
					return m.Logger().ErrorWith("获取文件信息失败", err, log.String("文件路径", lastFile1.Path))
				}
				lastFile1.Size = uint64(fileInfo.Size())
				return nil
			}) {
				return false
			}

			if lastFile1.StartTime != oldLastFile.StartTime ||
				lastFile1.EndTime != oldLastFile.EndTime ||
				lastFile1.Size != oldLastFile.Size {
				if !m.retryDo(interrupter, func() error {
					if res := m.fileTable().Updates(lastFile1); res.Error != nil {
						return m.Logger().ErrorWith("更新数据库中的文件信息失败", res.Error)
					}
					m.Logger().Info("更新数据库中的文件信息成功", log.Object("文件信息", lastFile1))
					return nil
				}) {
					return false
				}
				lock.LockDo(&m.filesLock, func() { *m.files.TailPointer() = *lastFile1 })
			}
		}
	}
	return true
}

func (m *DBMetaManager) loadLastIndex(interrupter chan struct{}) bool {
	return m.retryDo(interrupter, func() error {
		index := &Index{}
		if res := m.indexTable().Last(index); res.Error != nil {
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				m.Logger().Info("未找到最新的索引")
				return nil
			}
			m.Logger().ErrorWith("从数据库读取最新索引出错", res.Error)
			return res.Error
		}
		m.Logger().Info("从数据库读取最新索引成功")
		m.latestIndex = index
		return nil
	})
}

func (m *DBMetaManager) growEarliestIndexes() error {
	var need int
	var startSeq uint64
	var earliestLast *Index

	lock.LockDo(&m.indexLock, func() {
		need = m.earliestIndexes.Cap() - m.earliestIndexes.Len()
		if need == 0 {
			return
		}
		earliestLast = m.earliestIndexes.TailPointer()
		if earliestLast != nil {
			startSeq = earliestLast.SeqV
			earliestLast = earliestLast.clone()
		}
	})

	if need == 0 {
		return nil
	}

	var indexes []Index
	if res := m.indexTable().Where("seq > ?", startSeq).Order("seq").Limit(need).Find(&indexes); res.Error != nil {
		m.Logger().ErrorWith("从数据库读取索引出错", res.Error, log.Uint64("索引起始序列号", startSeq), log.Int("索引读取限制数量", need))
		return res.Error
	}
	m.Logger().Info("从数据库读取索引成功", log.Uint64("索引起始序列号", startSeq), log.Int("索引读取数量", len(indexes)))
	if len(indexes) > need {
		indexes = indexes[:need]
		m.Logger().Error("从数据库中读取的索引数量超过限制，丢弃多余的索引", log.Int("索引读取限制数量", need), log.Int("索引读取数量", len(indexes)))
	}
	var startTime int64
	if earliestLast != nil {
		startTime = earliestLast.EndV
	}
	indexChecker := storage.NewIndexChecker(startTime, 0, startSeq, 0)
	lock.LockDo(&m.indexLock, func() {
		for i := range indexes {
			index := &indexes[i]
			if indexChecker.Check(index) {
				m.earliestIndexes.Push(*index)
			}
		}
		m.loadableEarliestIndexes = m.earliestIndexes.Len()
	})
	if seqErrorCount := indexChecker.SeqErrorCount(); seqErrorCount > 0 {
		m.Logger().Error("发现错误的索引，错误原因为索引的序列号不大于上一个索引的序列号",
			log.Int("此错误原因的索引数量", seqErrorCount))
	}
	if timeErrorCount := indexChecker.TimeErrorCount(); timeErrorCount > 0 {
		m.Logger().Error("发现错误的索引，错误原因为索引的结束时间在开始时间之前，或是索引的开始时间在上一个索引的结束时间之前",
			log.Int("此错误原因的索引数量", timeErrorCount),
		)
	}
	return nil
}

func (m *DBMetaManager) loadEarliestIndexes(interrupter chan struct{}) bool {
	return m.retryDo(interrupter, m.growEarliestIndexes)
}

// updateChannel 更新数据库中的channel信息
func (m *DBMetaManager) updateChannel(interrupter chan struct{}) bool {
	channelInfo := lock.LockGet(&m.channelLock, func() *ChannelInfo { return m.channelInfo.Clone() })

	if interrupter != nil {
		if err := retry.MakeRetry(retry.Retry{
			Do: func() error {
				if res := m.dbManager.Table(ChannelInfoTableName).Select("timeOffset").Updates(channelInfo); res.Error != nil {
					m.Logger().ErrorWith("更新数据库中的通道信息失败", res.Error)
					return res.Error
				}
				return nil
			},
			MaxRetry:    -1,
			Interrupter: interrupter,
		}).Todo(); err == retry.InterruptedError {
			return true
		}
	} else {
		if res := m.dbManager.Table(ChannelInfoTableName).Select("timeOffset").Updates(channelInfo); res.Error != nil {
			m.Logger().ErrorWith("更新数据库中的通道信息失败", res.Error)
			return false
		}
	}

	m.Logger().Info("更新数据库中的通道信息成功", log.Object("通道信息", channelInfo))
	return false
}

// createIndex 向数据库中添加索引信息
func (m *DBMetaManager) createIndex(interrupter chan struct{}) bool {
	for {

		index, ok := lock.LockGetDouble(&m.indexLock, func() (*Index, bool) {
			first := m.needCreatedIndexes.HeadPointer()
			if first == nil {
				return nil, false
			}
			return first.clone(), true
		})
		if !ok {
			break
		}

		createCompletion := func() {
			index, _ := m.needCreatedIndexes.Pop()
			// 当确定所有索引都被加载到缓存中时，此时缓存一定未满，新创建的索引在添加到数据库之后将被添加
			// 到缓存中
			if m.loadableEarliestIndexes < m.earliestIndexes.Cap() {
				m.earliestIndexes.Push(index)
				m.loadableEarliestIndexes++
			}
		}

		if interrupter != nil {
			if err := retry.MakeRetry(retry.Retry{
				Do: func() error {
					if res := m.indexTable().Create(index); res.Error != nil {
						m.Logger().ErrorWith("向数据库中添加索引失败", res.Error)
						return res.Error
					}
					return nil
				},
				MaxRetry:    -1,
				Interrupter: interrupter,
			}).Todo(); err == retry.InterruptedError {
				return true
			}
		} else {
			if res := m.indexTable().Create(index); res.Error != nil {
				m.Logger().ErrorWith("向数据库中添加索引失败", res.Error)
				lock.LockDo(&m.indexLock, createCompletion)
				continue
			}
		}

		lock.LockDo(&m.indexLock, createCompletion)
		m.Logger().Debug("向数据库中添加索引成功", log.Object("索引", index))
	}
	return false
}

// deleteIndex 从数据库中删除索引信息
func (m *DBMetaManager) deleteIndex(interrupter chan struct{}) bool {
	for {
		index, ok := lock.LockGetDouble(&m.indexLock, func() (*Index, bool) {
			if m.needDeletedIndexes == 0 {
				return nil, false
			}
			return m.earliestIndexes.HeadPointer().clone(), true
		})
		if !ok {
			break
		}

		deleteCompletion := func() {
			m.earliestIndexes.Pop()
			m.needDeletedIndexes--
			// 在索引删除完成后，如果可能有未加载到缓存的索引，并且缓存中的索引不足容量的一半时，将数据
			// 库中加载索引填充到最早的索引缓存
			if m.loadableEarliestIndexes == m.earliestIndexes.Cap() && m.earliestIndexes.Len() < m.earliestIndexes.Cap()>>1 {
				m.growEarliestIndexesTaskList.Try(taskPkg.Func(func() { m.growEarliestIndexes() }))
			}
		}

		if interrupter != nil {
			if err := retry.MakeRetry(retry.Retry{
				Do: func() error {
					if res := m.indexTable().Delete(index); res.Error != nil {
						m.Logger().ErrorWith("从数据库中删除索引失败", res.Error)
						return res.Error
					}
					return nil
				},
				MaxRetry:    -1,
				Interrupter: interrupter,
			}).Todo(); err == retry.InterruptedError {
				return true
			}
		} else {
			if res := m.indexTable().Delete(index); res.Error != nil {
				m.Logger().ErrorWith("从数据库中删除索引失败", res.Error)
				lock.LockDo(&m.indexLock, deleteCompletion)
				continue
			}
		}

		lock.LockDo(&m.indexLock, deleteCompletion)
		m.Logger().Debug("从数据库中删除索引成功", log.Object("索引", index))
	}
	return false
}

// createFile 添加文件信息到数据库
func (m *DBMetaManager) createFile() {
	for needCreated := -1; ; {
		file, ok := lock.LockGetDouble(&m.filesLock, func() (*File, bool) {
			if needCreated == -1 {
				needCreated = m.needCreatedFiles.Len()
			}
			for needCreated != 0 {
				needCreated--
				if file := m.needCreatedFiles.RemoveFirst().Value(); file != nil {
					return file, true
				}
			}
			return nil, false
		})
		if !ok {
			break
		}

		if res := m.fileTable().Create(file); res.Error != nil {
			m.Logger().ErrorWith("向数据库中添加文件信息失败", res.Error)
			lock.LockDo(&m.filesLock, func() {
				if file := m.getFile(file.Seq, false); file != nil {
					m.needCreatedFiles.PutIfAbsent(file.Seq, file)
				}
			})
			continue
		}

		m.Logger().Info("向数据库中添加文件信息成功", log.Object("文件信息", file))
	}
}

// updateFile 更新数据库中的文件信息
func (m *DBMetaManager) updateFile() {
	for needUpdate := -1; ; {
		file, ok := lock.LockGetDouble(&m.filesLock, func() (*File, bool) {
			if needUpdate == -1 {
				needUpdate = m.needUpdatedFiles.Len()
			}
			for needUpdate != 0 {
				needUpdate--
				if file := m.needUpdatedFiles.RemoveFirst().Value(); file != nil {
					return file, true
				}
			}
			return nil, false
		})
		if !ok {
			break
		}

		if res := m.fileTable().Updates(file); res.Error != nil {
			m.Logger().ErrorWith("更新数据库中的文件信息失败", res.Error)
			lock.LockDo(&m.filesLock, func() {
				if file := m.getFile(file.Seq, false); file != nil {
					m.needUpdatedFiles.PutIfAbsent(file.Seq, file)
				}
			})
			continue
		}

		m.Logger().Info("更新数据库中的文件信息成功", log.Object("文件信息", file))
	}
}

// deleteFile 从数据库中删除文件信息
func (m *DBMetaManager) deleteFile() {

	for needDeleted := -1; ; {
		seq, ok := lock.LockGetDouble(&m.filesLock, func() (uint64, bool) {
			if needDeleted == -1 {
				needDeleted = m.needDeletedFiles.Len()
			}
			if needDeleted == 0 {
				return 0, false
			}
			needDeleted--
			return m.needDeletedFiles.RemoveFirst().Key(), true
		})
		if !ok {
			break
		}

		if res := m.fileTable().Where("seq = ?", seq).Delete(&File{}); res.Error != nil {
			m.Logger().ErrorWith("从数据库中删除文件信息失败", res.Error)
			lock.LockDo(&m.filesLock, func() { m.needDeletedFiles.PutIfAbsent(seq, struct{}{}) })
			continue
		}

		m.Logger().Info("从数据库中删除文件信息成功", log.Uint64("文件序列号", seq))
	}
}

func (m *DBMetaManager) doFileTask() {
	for _, task := range lock.LockGet(&m.filesLock, func() []func() {
		if m.fileTasks.Len() == 0 {
			return nil
		}
		tasks := make([]func(), m.fileTasks.Len())
		for i := 0; i < len(tasks); i++ {
			tasks[i] = m.fileTasks.RemoveFirst().Value()
		}
		return tasks
	}) {
		task()
	}
}

func (m *DBMetaManager) addFileTask(typ taskType, task func()) {
	if _, exist := m.fileTasks.PutIfAbsent(typ, task); !exist {
		if m.fileTasks.Len() == 1 {
			m.fileTaskExecutor.Async(taskPkg.Func(m.doFileTask))
		}
	}
}

func (m *DBMetaManager) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	dbWaiter := m.dbManager.StartedWaiter()
	m.dbManager.Background()

loop:
	for {
		select {
		case err := <-dbWaiter:
			if err != nil {
				return err
			}
			break loop
		case <-interrupter:
			m.dbManager.Close(nil)
		}
	}

	if !m.loadChannelInfo(interrupter) {
		m.Logger().Warn("加载通道信息被中断")
		return lifecycle.NewInterruptedError(m.DisplayName(), "启动")
	}
	if !m.ensureIndexTable(interrupter) {
		m.Logger().Warn("检查并创建索引表被中断")
		return lifecycle.NewInterruptedError(m.DisplayName(), "启动")
	}
	if !m.ensureFileTable(interrupter) {
		m.Logger().Warn("检查并创建文件信息表被中断")
		return lifecycle.NewInterruptedError(m.DisplayName(), "启动")
	}
	if !m.loadLastIndex(interrupter) {
		m.Logger().Warn("加载最新的索引被中断")
		return lifecycle.NewInterruptedError(m.DisplayName(), "启动")
	}
	if !m.loadEarliestIndexes(interrupter) {
		m.Logger().Warn("加载最早的索引被中断")
		return lifecycle.NewInterruptedError(m.DisplayName(), "启动")
	}
	if !m.loadFiles(interrupter) {
		m.Logger().Warn("加载文件信息被中断")
		return lifecycle.NewInterruptedError(m.DisplayName(), "启动")
	}

	m.channelUpdateTaskList.Start()
	m.addIndexTaskList.Start()
	m.deleteIndexTaskList.Start()
	m.growEarliestIndexesTaskList.Start()
	m.fileTaskExecutor.Start()

	return nil
}

func (m *DBMetaManager) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	<-interrupter

	m.channelUpdateTaskList.Close(nil)
	m.addIndexTaskList.Close(nil)
	m.deleteIndexTaskList.Close(nil)
	m.fileTaskExecutor.Close(nil)

	<-m.deleteIndexTaskList.ClosedWaiter()
	m.growEarliestIndexesTaskList.Close(nil)

	<-m.channelUpdateTaskList.ClosedWaiter()
	<-m.addIndexTaskList.ClosedWaiter()
	<-m.growEarliestIndexesTaskList.ClosedWaiter()
	<-m.fileTaskExecutor.ClosedWaiter()

	m.createFile()
	m.updateFile()
	m.deleteFile()
	m.createIndex(nil)
	m.deleteIndex(nil)

	return nil
}

func (m *DBMetaManager) DisplayName() string {
	return fmt.Sprintf("基于数据库的文件存储元数据管理器(%s)", m.channel.Name())
}

func (m *DBMetaManager) Channel() storage.Channel {
	return m.channel
}

func (m *DBMetaManager) FileChannel() storage.FileChannel {
	fc, _ := m.channel.(storage.FileChannel)
	return fc
}

func (m *DBMetaManager) DBManager() *dbPkg.DBManager {
	return m.dbManager
}

func (m *DBMetaManager) Offset(ms int64) int64 {
	return ms + m.channelInfo.TimeOffset
}

func (m *DBMetaManager) FirstIndex() (index storage.Index, loaded bool) {
	return lock.LockGetDouble(&m.indexLock, func() (storage.Index, bool) {
		if m.needDeletedIndexes != m.earliestIndexes.Len() {
			return m.earliestIndexes.Pointer(m.needDeletedIndexes).clone(), true
		} else if m.loadableEarliestIndexes == m.earliestIndexes.Cap() {
			return nil, false
		}
		return nil, true
	})
}

func (m *DBMetaManager) LastIndex() storage.Index {
	return lock.LockGet(&m.indexLock, func() storage.Index {
		if m.latestIndex == nil {
			return nil
		}
		return m.latestIndex.clone()
	})
}

func (m *DBMetaManager) NewIndex(start, end int64, size uint64, storageType uint32, state uint64) storage.Index {
	return &Index{
		StartV:       start,
		EndV:         end,
		SizeV:        size,
		StorageTypeV: storageType,
		StateV:       state,
	}
}

func (m *DBMetaManager) MakeIndexes(cap int) storage.Indexes {
	return make(Indexes, 0, cap)
}

//func (m *DBMetaManager) BuildIndex(index *Index) *Index {
//	index.SeqV = 1
//	last := m.latestIndex
//	if last != nil {
//		index.SeqV += last.SeqV
//	}
//	var offset int64
//	// 根据通道时时间偏移修正 index 的开始时间和结束时间
//	lock.LockDo(&m.channelLock, func() {
//		index.StartV += m.channelInfo.TimeOffset
//		index.EndV += m.channelInfo.TimeOffset
//		if last != nil {
//			if index.StartV < last.EndV {
//				// index 的开始时间在上一个 index 之前，时间发生跳变，需要修改通道时时间偏移
//				offset = last.EndV - index.StartV
//				m.channelInfo.TimeOffset += offset
//			}
//		}
//	})
//	if offset > 0 {
//		index.StartV += offset
//		index.EndV += offset
//		m.Logger().Warn("索引的开始时间在上一个索引的结束时间之前，修正当前索引的开始时间和结束时间", log.Int64("时间偏移量(ms)", offset))
//		m.channelUpdateTaskList.Try(taskPkg.Interrupted(m.updateChannel))
//	}
//	return index
//}

func (m *DBMetaManager) AddIndex(i storage.Index) error {
	index, is := i.(*Index)
	if !is {
		return errors.New("错误的索引类型")
	}
	index.SeqV = 1
	last := m.latestIndex
	if last != nil {
		index.SeqV += last.SeqV
	}
	var offset int64
	// 根据通道时时间偏移修正 index 的开始时间和结束时间
	lock.LockDo(&m.channelLock, func() {
		index.StartV += m.channelInfo.TimeOffset
		index.EndV += m.channelInfo.TimeOffset
		if last != nil {
			if index.StartV < last.EndV {
				// index 的开始时间在上一个 index 之前，时间发生跳变，需要修改通道时时间偏移
				offset = last.EndV - index.StartV
				m.channelInfo.TimeOffset += offset
			}
		}
	})
	if offset > 0 {
		index.StartV += offset
		index.EndV += offset
		m.Logger().Warn("索引的开始时间在上一个索引的结束时间之前，修正当前索引的开始时间和结束时间", log.Int64("时间偏移量(ms)", offset))
		m.channelUpdateTaskList.Try(taskPkg.Interrupted(m.updateChannel))
	}

	var put, drop = true, false
	lock.LockDo(&m.indexLock, func() {
		if m.needCreatedIndexes.Len()+1 >= m.needCreatedIndexes.Cap() {
			drop = true
			if m.needCreatedIndexes.Len() == m.needCreatedIndexes.Cap() {
				last := m.needCreatedIndexes.TailPointer()
				if last.StateV != IndexStateDropped {
					panic(fmt.Errorf("内部错误：如果缓存已满，那么最后一个索引的状态必须是 %s, 但实际为 %s",
						IndexStateString(IndexStateDropped),
						IndexStateString(last.StateV),
					))
				}
				last.EndV = index.EndV
				m.latestIndex = last.clone()
				put = false
			} else {
				// 第一个被丢弃的 index，标记并修改此 index
				index.FileSeqV = 0
				index.FileOffsetV = 0
				index.SizeV = 0
				index.StateV = IndexStateDropped
			}
		}
		if put {
			m.latestIndex = index.clone()
			if !m.needCreatedIndexes.Push(*index) {
				panic(errors.New("内部错误：前面已经判断缓存不满，此处缓存不可能无法添加索引到缓存中"))
			}
		}
	})

	if drop {
		m.Logger().Warn("所有缓存中的索引都没有添加到数据库，并且缓存已满，丢弃当前的索引", log.Uint64("索引序列号", index.SeqV))
	}
	if put {
		m.addIndexTaskList.Try(taskPkg.Interrupted(m.createIndex))
	}
	return nil
}

func (m *DBMetaManager) DeleteIndex() {
	lock.LockDo(&m.indexLock, func() {
		if m.needDeletedIndexes == m.earliestIndexes.Len() {
			return
		}
		m.needDeletedIndexes++
		m.deleteIndexTaskList.Try(taskPkg.Interrupted(m.deleteIndex))
	})
}

func (m *DBMetaManager) FindIndexes(start, end int64, limit int, is storage.Indexes) (storage.Indexes, error) {
	indexes := is.(Indexes)
	indexes = indexes[:0]
	if start >= end || limit == 0 {
		return indexes, nil
	}
	var extends []Index

	if lock.LockGet(&m.indexLock, func() (useCache bool) {
		head := m.needCreatedIndexes.HeadPointer()
		if head != nil && end > head.StartV {
			ei := sort.Search(m.needCreatedIndexes.Len(), func(i int) bool {
				return m.needCreatedIndexes.Pointer(i).StartV >= end
			})
			if start >= head.StartV {
				si := sort.Search(m.needCreatedIndexes.Len(), func(i int) bool {
					return m.needCreatedIndexes.Pointer(i).EndV > start
				})
				l := ei - si
				if l == 0 {
					return true
				}
				if limit > 0 && l > limit {
					ei -= l - limit
					l = limit
				}
				if cap(indexes) < l {
					indexes = make([]Index, 0, l)
				}
				prefix, suffix := m.needCreatedIndexes.Slice(si, ei)
				indexes = append(indexes, prefix...)
				indexes = append(indexes, suffix...)
				return true
			}
			if limit > 0 && ei > limit {
				ei = limit
			}
			extends = make([]Index, 0, ei)
			prefix, suffix := m.needCreatedIndexes.Slice(-1, ei)
			extends = append(extends, prefix...)
			extends = append(extends, suffix...)
			end = head.StartV
		}
		return false
	}) {
		return indexes, nil
	}

	var res *gorm.DB
	if limit < 0 {
		res = m.indexTable().Where("end > ? AND start < ?", start, end).Find(&indexes)
	} else {
		res = m.indexTable().Where("end > ? AND start < ?", start, end).Limit(limit).Find(&indexes)
	}
	fields := []log.Field{log.Int64("起始时间", start), log.Int64("结束时间", end), log.Int("查找索引总数", len(indexes))}
	if limit > 0 {
		fields = append(fields, log.Int("查找索引限制数量", limit))
	}
	if res.Error != nil {
		m.Logger().ErrorWith("从数据库查找索引出错", res.Error, fields...)
		return nil, res.Error
	}
	m.Logger().Info("从数据库查找索引成功", fields...)
	var endSeq uint64
	var endTime int64
	if len(extends) > 0 {
		endSeq = extends[0].SeqV
		endTime = extends[0].StartV
	}
	indexChecker := storage.NewIndexChecker(0, endTime, 0, endSeq)
	indexes = slice.SliceDelete(indexes, func(i int) bool {
		return !indexChecker.Check(&indexes[i])
	})
	if seqErrorCount := indexChecker.SeqErrorCount(); seqErrorCount > 0 {
		m.Logger().Error("发现错误的索引，错误原因为索引的序列号不大于上一个索引的序列号",
			log.Int("此错误原因的索引数量", seqErrorCount))
	}
	if timeErrorCount := indexChecker.TimeErrorCount(); timeErrorCount > 0 {
		m.Logger().Error("发现错误的索引，错误原因为索引的结束时间在开始时间之前，或是索引的开始时间在上一个索引的结束时间之前",
			log.Int("此错误原因的索引数量", timeErrorCount),
		)
	}
	if limit > 0 {
		if len(indexes) > limit {
			return indexes[:limit], nil
		}
		if len(indexes)+len(extends) > limit {
			return append(indexes, extends[:limit-len(indexes)]...), nil
		}
	}
	return append(indexes, extends...), nil
}

func (m *DBMetaManager) firstFile() *File {
	if file := m.files.HeadPointer(); file != nil {
		return file.Clone()
	}
	return nil
}

// FirstFile 从文件信息缓存中获取最新的文件信息
func (m *DBMetaManager) FirstFile() *File {
	if m.FileChannel() == nil {
		panic(NotFileChannelError)
	}
	return lock.LockGet(&m.filesLock, func() *File { return m.firstFile() })
}

func (m *DBMetaManager) lastFile() *File {
	if file := m.files.TailPointer(); file != nil {
		return file.Clone()
	}
	return nil
}

// LastFile 从文件信息缓存中获取最新的文件信息
func (m *DBMetaManager) LastFile() *File {
	if m.FileChannel() == nil {
		panic(NotFileChannelError)
	}
	return lock.LockGet(&m.filesLock, func() *File { return m.lastFile() })
}

func (m *DBMetaManager) findFile(seq uint64, include bool) int {
	if include {
		return sort.Search(m.files.Len(), func(i int) bool {
			return m.files.Pointer(i).Seq >= seq
		})
	}
	return sort.Search(m.files.Len(), func(i int) bool {
		return m.files.Pointer(i).Seq > seq
	})
}

func (m *DBMetaManager) getFile(seq uint64, clone bool) *File {
	if i := m.findFile(seq, true); i < m.files.Len() {
		if file := m.files.Pointer(i); file.Seq == seq {
			if clone {
				return file.Clone()
			}
			return file
		}
	}
	return nil
}

func (m *DBMetaManager) GetFile(seq uint64) *File {
	if m.FileChannel() == nil {
		panic(NotFileChannelError)
	}
	return lock.LockGet(&m.filesLock, func() *File { return m.getFile(seq, true) })
}

func (m *DBMetaManager) getFiles(files []File, start, end int64, limit int) []File {
	files = files[:0]
	if m.files.Len() == 0 || limit == 0 {
		return files
	}
	if start >= 0 && end >= 0 && start >= end {
		return files
	}
	si, ei := -1, -1
	if start >= 0 {
		si = m.findFile(uint64(start), true)
	}
	if end >= 0 {
		ei = m.findFile(uint64(end), true)
	}
	prefix, suffix := m.files.Slice(si, ei)
	if limit > 0 {
		if limit < len(prefix) {
			prefix = prefix[:limit]
			suffix = nil
		} else if limit < len(prefix)+len(suffix) {
			suffix = suffix[:limit-len(prefix)]
		}
	}
	l := len(prefix) + len(suffix)
	if cap(files) < l {
		files = make([]File, l)
	} else {
		files = files[:l]
	}
	copy(files[copy(files, prefix):], suffix)
	return files
}

func (m *DBMetaManager) GetFiles(files []File, start, end int64, limit int) []File {
	if m.FileChannel() == nil {
		panic(NotFileChannelError)
	}
	return lock.LockGet(&m.filesLock, func() []File { return m.getFiles(files, start, end, limit) })
}

// NewFile 创建一个文件信息结构体并填充一些基本信息（文件序列号，文件名，文件创建时间等）
func (m *DBMetaManager) NewFile(createTime int64, storageType uint32) *File {
	fc := m.FileChannel()
	if fc == nil {
		panic(NotFileChannelError)
	}
	seq := uint64(1)
	if last := m.LastFile(); last != nil {
		seq += last.Seq
	}
	channelInfo := lock.LockGet(&m.channelLock, func() *ChannelInfo { return m.channelInfo.Clone() })

	createTime += channelInfo.TimeOffset
	zone := time.FixedZone(channelInfo.Location, int(channelInfo.LocationOffset/1000))
	fileNameFormatter := fc.FileNameFormatter()
	if fileNameFormatter == nil {
		fileNameFormatter = storage.DefaultFileNameFormatter
	}
	name := fileNameFormatter(fc, seq, time.UnixMilli(createTime).In(zone), storageType)
	return &File{
		Seq: seq,
		//Name:       name,
		Path:       path.Join(fc.Directory(), name),
		CreateTime: createTime,
	}
}

func (m *DBMetaManager) AddFile(file *File) error {
	if m.FileChannel() == nil {
		panic(NotFileChannelError)
	}
	return lock.LockGet(&m.filesLock, func() error {
		last := m.files.TailPointer()
		if last != nil {
			if file.Seq <= last.Seq {
				panic(fmt.Errorf("内部错误：文件的序列号(%d)不大于上一个文件的序列号(%d)", file.Seq, last.Seq))
			}
		}
		if !m.files.Push(*file) {
			err := fmt.Errorf("文件数量超过了最大限制(%d)", m.files.Cap())
			m.Logger().Error(err.Error())
			return err
		}
		file = m.files.TailPointer()
		m.needCreatedFiles.Put(file.Seq, file)
		m.addFileTask(taskTypeFileCreate, m.createFile)
		return nil
	})
}

func (m *DBMetaManager) UpdateFile(file *File) {
	if m.FileChannel() == nil {
		panic(NotFileChannelError)
	}
	lock.LockDo(&m.filesLock, func() {
		old := m.getFile(file.Seq, false)
		if old == nil {
			return
		}
		*old = *file
		if m.needCreatedFiles.Has(file.Seq) {
			return
		}
		if _, exist := m.needUpdatedFiles.PutIfAbsent(file.Seq, old); !exist {
			m.addFileTask(taskTypeFileUpdate, m.updateFile)
		}
	})
}

func (m *DBMetaManager) DeleteFile(file *File) {
	if m.FileChannel() == nil {
		panic(NotFileChannelError)
	}
	lock.LockDo(&m.filesLock, func() {
		i := 0
		for files := m.files.Len(); i < files; i++ {
			if m.files.HeadPointer().Seq <= file.Seq {
				file, _ := m.files.Pop()
				if entry := m.needCreatedFiles.GetEntry(file.Seq); entry != nil {
					entry.SetValue(nil)
				}
				if entry := m.needUpdatedFiles.GetEntry(file.Seq); entry != nil {
					entry.SetValue(nil)
				}
				m.needDeletedFiles.Put(file.Seq, struct{}{})
			} else {
				break
			}
		}
		if i > 0 {
			m.addFileTask(taskTypeFileDelete, m.deleteFile)
		}
	})
}

var MetaTablesInfos = []dbPkg.TableInfo{
	{Name: ChannelInfoTableName, Comment: "存储通道信息表", Model: ChannelModel},
}
