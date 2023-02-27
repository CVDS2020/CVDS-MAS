package storage

import (
	"errors"
	"gitee.com/sy_183/common/lifecycle"
)

var ReadSessionClosedError = errors.New("存储读取会话已关闭")

type ReadSession interface {
	lifecycle.Lifecycle

	Read(buf []byte, index Index) (n int, err error)
}
