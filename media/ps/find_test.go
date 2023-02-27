package ps

import (
	"gitee.com/sy_183/common/uns"
	"testing"
)

func TestFind(t *testing.T) {
	ctx := findContext{sub: uns.StringToBytes("hello")}
	remain, ok := ctx.find(uns.StringToBytes("hell"))
	if ok {
		println(uns.BytesToString(ctx.buf) + uns.BytesToString(remain))
		return
	}
	remain, ok = ctx.find(uns.StringToBytes("io"))
	if ok {
		println(uns.BytesToString(ctx.buf) + uns.BytesToString(remain))
		return
	}
	remain, ok = ctx.find(uns.StringToBytes("o world"))
	if ok {
		println(uns.BytesToString(ctx.buf) + uns.BytesToString(remain))
		return
	}
}
