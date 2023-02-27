package main

import (
	"fmt"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/unit"
	defaultLogger "gitee.com/sy_183/cvds-mas/logger"
	"os"
	"sync"
	"time"
)

var logger = defaultLogger.Logger()

func main() {
	waiter := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		file := fmt.Sprintf("E:\\share\\test%d", i)
		waiter.Add(1)
		go func(logger *log.Logger) {
			fp, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
			if err != nil {
				logger.ErrorWith("打开文件失败", err)
				return
			}
			logger.Info("打开文件成功")
			defer func() {
				if err := fp.Close(); err != nil {
					logger.ErrorWith("关闭文件失败", err)
				}
				waiter.Done()
			}()
			for {
				n, err := fp.Write(make([]byte, 4*unit.MeBiByte))
				if err != nil {
					logger.ErrorWith("写文件失败", err)
					return
				}
				logger.Debug("写文件成功", log.Int("写入大小", n))
				_, err = fp.Seek(0, 0)
				if err != nil {
					logger.ErrorWith("定位文件失败", err)
					return
				}
				time.Sleep(time.Second * 4)
			}
		}(logger.Named(file))
		time.Sleep(time.Second * 2)
	}
	waiter.Wait()
}
