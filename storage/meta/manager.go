package meta

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/storage"
)

type Manager interface {
	lifecycle.Lifecycle

	DisplayName() string

	Channel() storage.Channel

	FileChannel() storage.FileChannel

	Offset(ms int64) int64

	FirstIndex() (index storage.Index, loaded bool)

	LastIndex() storage.Index

	NewIndex(start, end int64, size uint64, storageType uint32, state uint64) storage.Index

	MakeIndexes(cap int) storage.Indexes

	AddIndex(index storage.Index) error

	DeleteIndex()

	FindIndexes(start, end int64, limit int, indexes storage.Indexes) (storage.Indexes, error)

	FirstFile() *File

	LastFile() *File

	GetFile(seq uint64) *File

	GetFiles(files []File, start, end int64, limit int) []File

	NewFile(createTime int64, storageType uint32) *File

	AddFile(file *File) error

	UpdateFile(file *File)

	DeleteFile(file *File)

	log.LoggerProvider
}

type ManagerProvider func(channel storage.Channel, options ...Option) Manager
