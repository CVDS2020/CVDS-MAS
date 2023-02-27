package storage

import (
	"gitee.com/sy_183/common/pool"
	"io"
	"time"
)

type Frame interface {
	pool.Reference

	io.WriterTo

	Size() uint

	StartTime() time.Time

	EndTime() time.Time

	KeyFrame() bool

	MediaType() uint32

	StorageType() uint32
}
