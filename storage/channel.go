package storage

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/unit"
	"time"
)

type Channel interface {
	lifecycle.Lifecycle

	Cover() time.Duration

	SetCover(cover time.Duration) Channel

	BlockSize() unit.Size

	KeyFrameBlockSize() unit.Size

	MaxBlockSize() unit.Size

	MakeIndexes(cap int) Indexes

	FindIndexes(start, end int64, limit int, indexes Indexes) (Indexes, error)

	NewReadSession() (ReadSession, error)

	Write(frame Frame) bool

	Sync()

	log.LoggerProvider
}
