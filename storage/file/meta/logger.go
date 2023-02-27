package meta

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	defaultLogger "gitee.com/sy_183/cvds-mas/logger"
)

const Module = "storage.file.meta"

var logger log.AtomicLogger

func init() {
	logger.SetLogger(defaultLogger.Logger())
	config.Context().RegisterConfigReloadedCallback(func(_, nc *config.Config) {
		logger.SetLogger(config.LogConfig().MustBuild(Module).WithOptions(log.AddSubName(Module)))
	})
}

func Logger() *log.Logger {
	return logger.Logger()
}
