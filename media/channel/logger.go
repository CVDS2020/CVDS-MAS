package channel

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/media"
)

const (
	Module     = media.Module + ".channel"
	ModuleName = "媒体通道"
)

var logger log.AtomicLogger

func init() {
	config.InitModuleDefaultLogger(&logger, ModuleName)
	config.RegisterLoggerConfigReloadedCallback(&logger, Module, ModuleName)
}

func Logger() *log.Logger {
	return logger.Logger()
}
