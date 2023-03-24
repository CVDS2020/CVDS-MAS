package context

import (
	"fmt"
	"gitee.com/sy_183/common/errors"
	flagPkg "gitee.com/sy_183/common/flag"
	"io"
	"os"
	"path"
	"sync"
	"time"
)

// Context 文件上下文
type Context struct {
	// 文件路径
	file string
	// 文件指针
	fp *os.File
	// 当前的文件信息
	info os.FileInfo
	// 当前的文件指针偏移
	offset int64
	// 当前的文件大小
	size int64
	// 文件标志
	flag int

	// 文件生命周期携带的上下文数据
	context any

	mu sync.Mutex
}

func (c *Context) File() string {
	return c.file
}

func (c *Context) Fp() *os.File {
	return c.fp
}

func (c *Context) Info() os.FileInfo {
	return c.info
}

func (c *Context) Offset() int64 {
	return c.offset
}

func (c *Context) Size() int64 {
	return c.size
}

func (c *Context) Flag() int {
	return c.flag
}

func (c *Context) IsAppend() bool {
	return flagPkg.TestFlag(c.flag, os.O_APPEND)
}

func (c *Context) IsTruncate() bool {
	return flagPkg.TestFlag(c.flag, os.O_TRUNC)
}

func (c *Context) IsDirect() bool {
	return flagPkg.TestFlag(c.flag, O_DIRECT)
}

func (c *Context) IsSync() bool {
	return flagPkg.TestFlag(c.flag, os.O_SYNC)
}

func (c *Context) FileContext() any {
	return c.context
}

func OpenFile(req *OpenRequest) *OpenResponse {
	res := &OpenResponse{Request: req}
	start := time.Now()

	// 打开文件
	fp, err := os.OpenFile(req.File, req.Flag, 0644)
	if err != nil && flagPkg.TestFlag(req.Flag, os.O_CREATE) {
		if err = os.MkdirAll(path.Dir(req.File), 0755); err == nil {
			fp, err = os.OpenFile(req.File, req.Flag, 0644)
		}
	}
	// 计算打开文件花费的时间
	res.OpenElapse = time.Since(start)
	start = start.Add(res.OpenElapse)
	if err != nil {
		res.Err = &OpenError{Err{err}}
		return res
	}

	// 获取文件信息
	info, err := fp.Stat()
	// 计算获取文件信息花费的时间
	res.GetInfoElapse = time.Since(start)
	start = start.Add(res.GetInfoElapse)
	if err != nil {
		res.Err = &GetInfoError{Err{err}}
		// 获取不到文件信息，关闭文件
		if err := fp.Close(); err != nil {
			res.Err = errors.Append(res.Err, &CloseError{Err{err}})
		}
		// 计算关闭文件花费的时间
		res.CloseElapse = time.Since(start)
		start = start.Add(res.CloseElapse)
		return res
	}

	// 获取当前文件指针偏移
	var offset int64
	if !flagPkg.TestFlag(req.Flag, os.O_APPEND) {
		off, err := fp.Seek(0, 1)
		// 计算获取文件指针位置花费的时间
		res.SeekElapse = time.Since(start)
		start = start.Add(res.SeekElapse)
		if err != nil {
			res.Err = &SeekError{Err{err}}
			if err := fp.Close(); err != nil {
				res.Err = errors.Append(res.Err, &CloseError{Err{err}})
			}
			// 计算关闭文件花费的时间
			res.CloseElapse = time.Since(start)
			start = start.Add(res.CloseElapse)
			return res
		}
		offset = off
	} else {
		// 以追加方式打开文件，无法调用 Seek 函数确定文件指针位置，使用文件大小作为当前文件指针偏移配置
		offset = info.Size()
	}

	res.Context = &Context{
		file:    req.File,
		fp:      fp,
		info:    info,
		offset:  offset,
		size:    info.Size(),
		flag:    req.Flag,
		context: req.FileContext,
	}

	res.FileInfo = info
	res.Offset = offset

	req.Future.Response(res)

	return res
}

func (c *Context) Write(req *WriteRequest) *WriteResponse {
	res := &WriteResponse{Request: req, Context: c}
	start := time.Now()

	c.mu.Lock()
	defer func() {
		future := req.Future
		if future.Callback != nil {
			future.Callback(res)
		}
		c.mu.Unlock()
		if future.Channel != nil {
			future.Channel <- res
		}
	}()

	if c.fp == nil {
		panic(fmt.Errorf("文件(%s)已经被关闭", c.file))
	}

	res.FileInfo = c.info
	res.Offset = c.offset

	// 如果指定了偏移写并且文件不是以追加方式打开，则先使用 Seek 函数移动函数指针
	if seek := req.Seek; seek != nil && !c.IsAppend() {
		offset, err := c.fp.Seek(seek.Offset, seek.Whence)
		// 计算修改文件指针位置花费的时间
		res.SeekElapse = time.Since(start)
		start = start.Add(res.SeekElapse)
		if err != nil {
			res.Err = &SeekError{Err{err}}
			return res
		}
		c.offset = offset
		if c.offset > c.size {
			c.size = c.offset
		}
		res.Offset = c.offset
	}

	// 将数据写入文件，并计算新的文件指针偏移和文件大小
	if len(req.Data) > 0 {
		n, err := c.fp.Write(req.Data)
		// 计算文件写入花费的时间
		res.WriteElapse = time.Since(start)
		start = start.Add(res.WriteElapse)
		if n > 0 {
			c.offset += int64(n)
			if c.offset > c.size {
				c.size = c.offset
			}
			res.Offset = c.offset
			res.Size = int64(n)
		}
		if err != nil {
			res.Err = &WriteError{Err{err}}
			return res
		}
	}

	// 如果指定了写入后重新获取文件信息，则更新文件信息
	if req.GetInfo {
		info, err := c.fp.Stat()
		// 计算更新文件信息花费的时间
		res.GetInfoElapse = time.Since(start)
		start = start.Add(res.GetInfoElapse)
		if err != nil {
			res.Err = &GetInfoError{Err{err}}
			return res
		}
		c.info = info
		res.FileInfo = info
	}
	return res
}

func (c *Context) Read(req *ReadRequest) *ReadResponse {
	res := &ReadResponse{Request: req, Context: c}
	start := time.Now()

	c.mu.Lock()
	defer func() {
		future := req.Future
		if future.Callback != nil {
			future.Callback(res)
		}
		c.mu.Unlock()
		if future.Channel != nil {
			future.Channel <- res
		}
	}()

	if c.fp == nil {
		panic(fmt.Errorf("file(%s) has been closed", c.file))
	}

	res.Offset = c.offset

	// 如果指定了偏移写并且文件不是以追加方式打开，则先使用 Seek 函数移动函数指针
	if seek := req.Seek; seek != nil && !c.IsAppend() {
		offset, err := c.fp.Seek(seek.Offset, seek.Whence)
		// 计算修改文件指针位置花费的时间
		res.SeekElapse = time.Since(start)
		start = start.Add(res.SeekElapse)
		if err != nil {
			res.Err = &SeekError{Err{err}}
			return res
		}
		c.offset = offset
		if c.offset > c.size {
			c.size = c.offset
		}
		res.Offset = c.offset
	}

	if len(req.Buf) > 0 {
		reader := func(r io.Reader, buf []byte) (n int, err error) { return r.Read(buf) }
		if req.Full {
			reader = io.ReadFull
		}
		n, err := reader(c.fp, req.Buf)
		res.ReadElapse = time.Since(start)
		if n > 0 {
			c.offset += int64(n)
			if c.offset > c.size {
				c.size = c.offset
			}
			res.Offset = c.offset
		}
		if err != nil {
			res.Err = &ReadError{Err{err}}
		}
		res.Data = req.Buf[:n]
	}
	return res
}

func (c *Context) Close(req *CloseRequest) *CloseResponse {
	res := &CloseResponse{Request: req, Context: c}
	start := time.Now()

	c.mu.Lock()
	defer func() {
		future := req.Future
		if future.Callback != nil {
			future.Callback(res)
		}
		c.mu.Unlock()
		if future.Channel != nil {
			future.Channel <- res
		}
	}()

	err := c.fp.Close()
	c.fp = nil
	res.CloseElapse = time.Since(start)
	if err != nil {
		res.Err = &CloseError{Err{err}}
	}
	return res
}

func Remove(req *RemoveRequest) *RemoveResponse {
	res := &RemoveResponse{Request: req}
	start := time.Now()

	defer req.Future.Response(res)
	err := os.Remove(req.File)
	res.DeleteElapse = time.Since(start)
	if err != nil && !os.IsNotExist(err) {
		res.Err = &RemoveError{Err{err}}
	}
	return res
}
