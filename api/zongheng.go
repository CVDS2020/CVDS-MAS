package api

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/cvds-mas/zongheng"
	"github.com/gin-gonic/gin"
)

type ZongHeng struct {
	channelManager *zongheng.ChannelManager
}

func newZongHeng() *ZongHeng {
	z := &ZongHeng{channelManager: zongheng.GetChannelManager()}
	return z
}

func (z *ZongHeng) Sync(ctx *gin.Context) {
	z.channelManager.Sync()
	responseSuccess(ctx, nil)
}

var GetZongHeng = component.NewPointer(newZongHeng).Get
