package main

import (
	"encoding/binary"
	"fmt"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/uns"
	defaultLogger "gitee.com/sy_183/cvds-mas/logger"
	"syscall"
	"time"
	"unsafe"
)

var logger = defaultLogger.Logger()

func main() {
	hmap, err := syscall.CreateFileMapping(syscall.InvalidHandle, nil, syscall.PAGE_READWRITE, 0, 4096, assert.Must(syscall.UTF16PtrFromString("mem_test")))
	if err != nil {
		logger.ErrorWith("创建共享内存失败", err)
		return
	}

	defer func() {
		if err := syscall.CloseHandle(hmap); err != nil {
			logger.ErrorWith("关闭共享内存失败", err)
		}
	}()

	addr, err := syscall.MapViewOfFile(hmap, syscall.FILE_MAP_WRITE|syscall.FILE_MAP_READ, 0, 0, 0)
	if err != nil {
		logger.ErrorWith("映射共享内存失败", err)
		return
	}

	defer func() {
		if err := syscall.UnmapViewOfFile(addr); err != nil {
			logger.ErrorWith("取消映射共享内存失败", err)
		}
	}()

	data := uns.MakeBytes(unsafe.Pointer(addr), 4096, 4096)

	buf := make([]byte, 8)
	for i := 0; i < 10000; i++ {
		s := fmt.Sprintf("hello(%d)", i)
		binary.BigEndian.PutUint64(buf, uint64(len(s)))
		copy(data[copy(data, buf):], s)
		logger.Info("向共享中写入数据", log.String("数据", s))
		time.Sleep(time.Second)
	}
}
