package rtp

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
)

func defaultOnStarting(name string, logger log.LoggerProvider) lifecycle.OnStartingFunc {
	return func(lifecycle.Lifecycle) {
		logOnStarting(name, logger)
	}
}

func defaultOnStarted(name string, logger log.LoggerProvider) lifecycle.OnStartedFunc {
	return func(_ lifecycle.Lifecycle, err error) {
		logOnStarted(name, logger, err)
	}
}

func defaultOnClose(name string, logger log.LoggerProvider) lifecycle.OnStartedFunc {
	return func(_ lifecycle.Lifecycle, err error) {
		logOnClose(name, logger, err)
	}
}

func defaultOnClosed(name string, logger log.LoggerProvider) lifecycle.OnStartedFunc {
	return func(_ lifecycle.Lifecycle, err error) {
		logOnClosed(name, logger, err)
	}
}
