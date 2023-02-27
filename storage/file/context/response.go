package context

import (
	"os"
	"time"
)

type OpenResponse struct {
	// 对应的更新请求
	Request *OpenRequest
	// 新文件的上下文
	*Context

	// 新文件的文件信息
	FileInfo os.FileInfo
	// 新文件的文件指针偏移
	Offset int64

	// 打开文件花费的时间
	OpenElapse time.Duration
	// 获取文件信息花费的时间
	GetInfoElapse time.Duration
	// 获取文件指针位置花费的时间
	SeekElapse time.Duration
	// 关闭文件花费的时间
	CloseElapse time.Duration

	// 打开错误
	Err error
}

func (r *OpenResponse) Elapse() time.Duration {
	return r.OpenElapse + r.GetInfoElapse + r.SeekElapse + r.CloseElapse
}

type ReadResponse struct {
	// 对应的读请求
	Request *ReadRequest
	// 文件的上下文
	*Context

	// 文件读取到的数据，是请求缓冲区的切片
	Data []byte
	// 文件读取完成后的文件指针偏移
	Offset int64

	// 修改文件指针位置花费的时间
	SeekElapse time.Duration
	// 读取文件花费的时间
	ReadElapse time.Duration

	// 读取错误
	Err error
}

func (r *ReadResponse) Elapse() time.Duration {
	return r.SeekElapse + r.ReadElapse
}

type WriteResponse struct {
	// 对应的写请求
	Request *WriteRequest
	// 文件的上下文
	*Context

	// 文件写入完成后的文件信息，如果请求时未指定更新文件信息，则为上一次更新时的文件信息
	FileInfo os.FileInfo
	// 文件写入的大小
	Size int64
	// 文件写入完成后的文件指针偏移
	Offset int64

	// 修改文件指针位置花费的时间
	SeekElapse time.Duration
	// 文件写入花费的时间
	WriteElapse time.Duration
	// 获取文件信息花费的时间
	GetInfoElapse time.Duration

	// 写入错误
	Err error
}

func (r *WriteResponse) Elapse() time.Duration {
	return r.SeekElapse + r.WriteElapse + r.GetInfoElapse
}

type CloseResponse struct {
	// 对应的关闭请求
	Request *CloseRequest
	// 文件的上下文
	*Context
	// 关闭文件花费的时间
	CloseElapse time.Duration
	// 关闭错误
	Err error
}

type RemoveResponse struct {
	// 对应的删除请求
	Request *RemoveRequest
	// 删除文件花费的时间
	DeleteElapse time.Duration
	// 删除错误
	Err error
}
