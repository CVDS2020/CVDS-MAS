package main

import (
	"fmt"
	"gitee.com/sy_183/common/log"
	syssvc "gitee.com/sy_183/common/system/service"
	"gitee.com/sy_183/cvds-mas/app"
	"gitee.com/sy_183/cvds-mas/config"
	defaultLogger "gitee.com/sy_183/cvds-mas/logger"
	"github.com/common-nighthawk/go-figure"
	"os"
)

var logger log.AtomicLogger

func Logger() *log.Logger {
	return logger.Logger()
}

func init() {
	logger.SetLogger(defaultLogger.Logger())
	config.Context().RegisterConfigReloadedCallback(func(_, nc *config.Config) {
		logger.SetLogger(nc.Log.MustBuild("main"))
	})
}

func printFigure() {
	defer func() {
		if e := recover(); e != nil {
			if err, is := e.(error); is {
				Logger().Fatal("print figure error", log.Error(err))
			} else {
				Logger().Fatal(fmt.Sprintf("print figure error: %v", e))
			}
		}
	}()
	figureConfig := config.FigureConfig()
	if figureConfig.Color == "" {
		figure.NewFigure(figureConfig.Phrase, figureConfig.Font, figureConfig.Strict).Print()
	} else {
		figure.NewColorFigure(figureConfig.Phrase, figureConfig.Font, figureConfig.Color, figureConfig.Strict).Print()
	}
}

func main() {
	printFigure()
	os.Exit(syssvc.New(config.ServiceConfig().Name, app.GetApp(),
		syssvc.ErrorCallback(syssvc.LogErrorCallback(Logger()))).Run())
}
