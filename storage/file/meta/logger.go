package meta

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
)

const (
	Module     = "storage.file.meta"
	ModuleName = "文件存储元数据管理器"
)

var logger log.AtomicLogger

func init() {
	config.InitModuleDefaultLogger(&logger, ModuleName)
	config.RegisterLoggerConfigReloadedCallback(&logger, Module, ModuleName)
}

func Logger() *log.Logger {
	return logger.Logger()
}
