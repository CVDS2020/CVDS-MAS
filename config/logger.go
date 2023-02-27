package config

import (
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/log"
	logConfig "gitee.com/sy_183/common/log/config"
)

const (
	Module     = "config"
	ModuleName = "配置管理器"
)

var (
	defaultLogger = config.MustHandleWith(new(logConfig.LoggerConfig)).MustBuild()
	logger        log.AtomicLogger
)

func init() {
	InitModuleDefaultLogger(&logger, ModuleName)
	context.RegisterConfigReloadedCallback(LoggerConfigReloadedCallback(&logger, Module, ModuleName))
}

func InitModuleDefaultLogger(logger log.LoggerProvider, moduleName string) {
	logger.SetLogger(defaultLogger.Named(moduleName))
}

func logConfigError(logger *log.Logger, moduleName string, err error) {
	logger.Warn("日志配置失败", log.String("模块", moduleName), log.NamedError("错误", err))
}

func logDefaultConfigError(logger *log.Logger, err error) {
	logger.Warn("日志默认配置失败", log.NamedError("错误", err))
}

func ReloadModuleLogger(oc, nc *Config, logger log.LoggerProvider, module, moduleName string) {
	if nl, err := nc.Log.Build(module); err == nil {
		// 使用模块指定的日志配置
		logger.SetLogger(nl.Named(moduleName))
	} else {
		// 模块指定的日志配置失败，如果是第一次初始化，则使用配置文件中默认的日志配置，否则不更改日志配置
		if oc == nil {
			if nl, err := nc.Log.Build(module); err == nil {
				logger.SetLogger(nl.Named(moduleName))
				logConfigError(logger.Logger(), moduleName, err)
			} else {
				// 如果配置文件中默认的日志配置也失败，则使用默认的日志配置
				logger.CompareAndSwapLogger(nil, defaultLogger.Named(moduleName))
				logConfigError(logger.Logger(), moduleName, err)
				logDefaultConfigError(logger.Logger(), err)
			}
		} else {
			logConfigError(logger.Logger(), moduleName, err)
		}
	}
}

func InitModuleLogger(logger log.LoggerProvider, module, moduleName string) {
	ReloadModuleLogger(nil, GlobalConfig(), logger, module, moduleName)
}

func RegisterLoggerConfigReloadedCallback(logger log.LoggerProvider, module, moduleName string) {
	context.RegisterConfigReloadedCallback(func(oc, nc *Config) {
		ReloadModuleLogger(oc, nc, logger, module, moduleName)
	})
}

func LoggerConfigReloadedCallback(logger log.LoggerProvider, module, moduleName string) func(oc, nc *Config) {
	return func(oc, nc *Config) { ReloadModuleLogger(oc, nc, logger, module, moduleName) }
}

func Logger() *log.Logger {
	return logger.Logger()
}

func DefaultLogger() *log.Logger {
	return defaultLogger
}
