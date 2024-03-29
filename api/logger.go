package api

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
)

const (
	Module     = "api"
	ModuleName = "API服务"
)

var logger log.AtomicLogger

func init() {
	config.InitModuleDefaultLogger(&logger, ModuleName)
	config.RegisterLoggerConfigReloadedCallback(&logger, Module, ModuleName)
}

func Logger() *log.Logger {
	return logger.Logger()
}
