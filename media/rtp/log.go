package rtp

import (
	"gitee.com/sy_183/common/log"
)

func logOnStarting(name string, logger log.LoggerProvider) {
	if logger != nil {
		logger.Logger().Info(name + "正在启动")
	}
}

func logOnStarted(name string, logger log.LoggerProvider, err error) {
	if logger != nil {
		if err != nil {
			logger.Logger().ErrorWith(name+"启动失败", err)
		} else {
			logger.Logger().Info(name + "启动成功")
		}
	}
}

func logOnClose(name string, logger log.LoggerProvider, err error) {
	if logger != nil {
		if err != nil {
			logger.Logger().ErrorWith(name+"执行关闭操作失败", err)
		} else {
			logger.Logger().Info(name + "执行关闭操作成功")
		}
	}
}

func logOnClosed(name string, logger log.LoggerProvider, err error) {
	if logger != nil {
		if err != nil {
			logger.Logger().ErrorWith(name+"出现错误并退出", err)
		} else {
			logger.Logger().Info(name + "已退出")
		}
	}
}
