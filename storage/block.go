package storage

import (
	"gitee.com/sy_183/common/pool"
	"io"
)

type RefWriter interface {
	pool.Reference

	io.WriterTo
}
