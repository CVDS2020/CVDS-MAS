package file

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
)

type MetaManager interface {
	lifecycle.Lifecycle

	FirstIndex() *Index

	LastIndex() *Index

	BuildIndex(index *Index) *Index

	AddIndex(index *Index) error

	DeleteIndex()

	FindIndexes(start, end int64, limit int, indexes Indexes) (Indexes, error)

	FirstFile() *File

	LastFile() *File

	GetFile(seq uint64) *File

	NewFile(createTime int64, mediaType, storageType uint32) *File

	AddFile(file *File) error

	UpdateFile(file *File)

	DeleteFile(file *File)

	log.LoggerProvider
}

const (
	DefaultLastIndexMaxCache  = 65535
	DefaultFirstIndexMaxCache = 64
)
