package main

import (
	"C"
	"encoding/binary"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/uns"
	"gitee.com/sy_183/cvds-mas/config"
	"syscall"
	"time"
	"unsafe"
)

var logger = config.DefaultLogger()

func main() {
	//modkernel32 := syscall.NewLazyDLL("kernel32.dll")
	//procOpenFileMappingW := modkernel32.NewProc("OpenFileMappingW")

	//r0, _, err := procOpenFileMappingW.Call(3, uintptr(syscall.FILE_MAP_WRITE|syscall.FILE_MAP_READ), uintptr(unsafe.Pointer(assert.Must(syscall.UTF16PtrFromString("mem_test")))))
	//hmap := syscall.Handle(r0)
	//if hmap == 0 {
	//	logger.ErrorWith("打开共享内存失败", err)
	//	return
	//}

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

	for i := 0; i < 100; i++ {
		l := binary.BigEndian.Uint64(data)
		if l < 100 {
			logger.Info("从共享内存中读取数据", log.String("数据", string(data[8:8+l])))
		}
		time.Sleep(time.Second)
	}
}
