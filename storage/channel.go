package storage

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"io"
	"time"
)

type Channel interface {
	lifecycle.Lifecycle

	Name() string

	DisplayName() string

	Cover() time.Duration

	SetCover(cover time.Duration) Channel

	WriteBufferSize() uint

	MakeIndexes(cap int) Indexes

	FindIndexes(start, end int64, limit int, indexes Indexes) (Indexes, error)

	NewReadSession() (ReadSession, error)

	Write(writer io.WriterTo, size uint, start, end time.Time, storageType uint32, sync bool) error

	WriteStreamLost(start, end time.Time) error

	Sync() error

	log.LoggerProvider
}
