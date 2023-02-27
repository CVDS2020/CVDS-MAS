package api

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/utils"
	"gitee.com/sy_183/cvds-mas/api/bean"
	"gitee.com/sy_183/cvds-mas/config"
	"github.com/gin-gonic/gin"
	"net/http"
)

const (
	SysModule     = "api.sys"
	SysModuleName = "系统管理器API服务"
)

type Sys struct {
	RestartSignal      chan struct{}
	StopSignal         chan struct{}
	ReloadConfigSignal chan struct{}
	log.AtomicLogger
}

func newSys() *Sys {
	s := &Sys{
		ReloadConfigSignal: make(chan struct{}, 1),
		RestartSignal:      make(chan struct{}, 1),
		StopSignal:         make(chan struct{}, 1),
	}
	config.InitModuleLogger(s, SysModule, SysModuleName)
	config.RegisterLoggerConfigReloadedCallback(s, SysModule, SysModuleName)
	return s
}

func (s *Sys) ReloadConfig(ctx *gin.Context) {
	s.Logger().Info("配置文件重新加载中...")
	if utils.ChanTryPush(s.ReloadConfigSignal, struct{}{}) {
		ctx.JSON(http.StatusOK, bean.Result[any]{
			Code: http.StatusOK,
			Msg:  "success",
		})
		return
	}
	ctx.AbortWithStatusJSON(http.StatusOK, bean.Result[any]{
		Code: http.StatusBadRequest,
		Msg:  "busy",
	})
	return
}

func (s *Sys) Restart(ctx *gin.Context) {
	s.Logger().Info("系统正在重启中...")
	if utils.ChanTryPush(s.RestartSignal, struct{}{}) {
		ctx.JSON(http.StatusOK, bean.Result[any]{
			Code: http.StatusOK,
			Msg:  "success",
		})
		return
	}
	ctx.AbortWithStatusJSON(http.StatusOK, bean.Result[any]{
		Code: http.StatusBadRequest,
		Msg:  "busy",
	})
	return
}

func (s *Sys) Stop(ctx *gin.Context) {
	s.Logger().Info("系统正在关闭中...")
	if utils.ChanTryPush(s.StopSignal, struct{}{}) {
		ctx.JSON(http.StatusOK, bean.Result[any]{
			Code: http.StatusOK,
			Msg:  "success",
		})
		return
	}
	ctx.AbortWithStatusJSON(http.StatusOK, bean.Result[any]{
		Code: http.StatusBadRequest,
		Msg:  "busy",
	})
	return
}

var GetSys = component.NewPointer(newSys).Get
