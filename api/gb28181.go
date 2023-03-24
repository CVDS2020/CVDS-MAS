package api

import (
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/gb28181"
	gbErrors "gitee.com/sy_183/cvds-mas/gb28181/errors"
	modelPkg "gitee.com/sy_183/cvds-mas/gb28181/model"
	"gitee.com/sy_183/cvds-mas/gb28181/sip"
	"github.com/gin-gonic/gin"
)

const (
	GB28181Module     = Module + ".gb28181"
	GB28181ModuleName = "国标管理器API服务"
)

type GB28181 struct {
	storageConfigManager *gb28181.StorageConfigManager
	channelManager       *gb28181.ChannelManager
	log.AtomicLogger
}

func newGB28181() *GB28181 {
	g := &GB28181{
		storageConfigManager: gb28181.GetStorageConfigManager(),
		channelManager:       gb28181.GetChannelManager(),
	}
	config.InitModuleLogger(g, GB28181Module, GB28181ModuleName)
	config.RegisterLoggerConfigReloadedCallback(g, GB28181Module, GB28181ModuleName)
	return g
}

func (g *GB28181) GetStorageConfig(ctx *gin.Context) {
	name := ctx.Query("name")
	if name == "" {
		responseError(ctx, &gbErrors.ArgumentMissingError{Arguments: []string{"name"}}, true)
		return
	}

	if storageConfig, err := g.storageConfigManager.GetStorageConfig(name); err != nil {
		responseError(ctx, err, false)
		return
	} else {
		responseSuccess(ctx, storageConfig)
	}
}

func (g *GB28181) AddStorageConfig(ctx *gin.Context) {
	model := modelPkg.StorageConfig{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if err := g.storageConfigManager.AddStorageConfig(&model); err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, nil)
}

func (g *GB28181) DeleteStorageConfig(ctx *gin.Context) {
	name := ctx.Query("name")
	if name == "" {
		responseError(ctx, &gbErrors.ArgumentMissingError{Arguments: []string{"name"}}, true)
		return
	}

	if err := g.storageConfigManager.DeleteStorageConfig(name); err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, nil)
}

func (g *GB28181) ProxyInvite(ctx *gin.Context) {
	request := sip.Request{}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if err := request.Check(); err != nil {
		responseError(ctx, err, true)
		return
	}

	responseSuccess(ctx, g.channelManager.ProcessInvite(&request))
}

func (g *GB28181) ProxyAck(ctx *gin.Context) {
	request := sip.Request{}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if err := request.Check(); err != nil {
		responseError(ctx, err, true)
		return
	}

	g.channelManager.ProcessAck(&request)
	responseSuccess(ctx, nil)
}

func (g *GB28181) ProxyBye(ctx *gin.Context) {
	request := sip.Request{}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if err := request.Check(); err != nil {
		responseError(ctx, err, true)
		return
	}

	responseSuccess(ctx, g.channelManager.ProcessBye(&request))
}

func (g *GB28181) ProxyInfo(ctx *gin.Context) {
	request := sip.Request{}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if err := request.Check(); err != nil {
		responseError(ctx, err, true)
		return
	}

	responseSuccess(ctx, g.channelManager.ProcessInfo(&request))
}

var GetGB28181 = component.NewPointer(newGB28181).Get
