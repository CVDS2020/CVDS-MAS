package api

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/cvds-mas/media/rtsp2"
	"github.com/gin-gonic/gin"
)

type RTSP struct {
	server *rtsp2.Server
}

func newRTSP() *RTSP {
	r := &RTSP{server: rtsp2.GetServer()}
	return r
}

func (r *RTSP) AddChannel(ctx *gin.Context) {
	path, channel := ctx.Query("path"), ctx.Query("channel")
	if path == "" || channel == "" {
		responseError(ctx, errors.NewArgumentMissing("path", "channel"), true)
		return
	}

	r.server.AddChannel(path, channel)
	responseSuccess(ctx, nil)
}

func (r *RTSP) GetChannel(ctx *gin.Context) {
	path := ctx.Query("path")
	if path == "" {
		responseError(ctx, errors.NewArgumentMissing("path"), true)
		return
	}

	channel := r.server.GetChannel(path)
	responseSuccess(ctx, channel)
}

func (r *RTSP) PathChannels(ctx *gin.Context) {
	responseSuccess(ctx, r.server.PathChannels())
}

func (r *RTSP) DeleteChannel(ctx *gin.Context) {
	path := ctx.Query("path")
	if path == "" {
		responseError(ctx, errors.NewArgumentMissing("path"), true)
		return
	}

	r.server.DeleteChannel(path)
	responseSuccess(ctx, nil)
}

var GetRTSP = component.NewPointer(newRTSP).Get
