package file

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/storage"
)

const (
	Module     = storage.Module + ".file"
	ModuleName = "文件存储"
)

var logger log.AtomicLogger

func init() {
	config.InitModuleDefaultLogger(&logger, ModuleName)
	config.RegisterLoggerConfigReloadedCallback(&logger, Module, ModuleName)
}

func Logger() *log.Logger {
	return logger.Logger()
}
