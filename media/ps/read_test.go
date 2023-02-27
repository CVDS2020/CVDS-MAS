package ps

import (
	"gitee.com/sy_183/common/uns"
	"testing"
)

func TestRead(t *testing.T) {
	ctx := readContext{
		len: 10,
		buf: nil,
	}
	remain, ok := ctx.read(uns.StringToBytes("hello1111111111"))
	if ok {
		println(uns.BytesToString(ctx.buf))
		println(uns.BytesToString(remain))
		return
	}
	remain, ok = ctx.read(uns.StringToBytes(" world"))
	if ok {
		println(uns.BytesToString(ctx.buf))
		println(uns.BytesToString(remain))
		return
	}
}
