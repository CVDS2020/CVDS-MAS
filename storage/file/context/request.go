package context

import (
	"gitee.com/sy_183/cvds-mas/storage"
)

type OpenRequest struct {
	// 文件路径
	File string

	// 打开文件的标志
	Flag int
	// 当打开文件标志中包含 os.O_CREATE 标志，但文件所在目录不存在时，自动创建
	// 父目录
	Mkdir bool

	// 请求生命周期携带的上下文数据
	Context any
	// 文件生命周期携带的上下文数据
	FileContext any

	// 文件打开完成的通知通道
	Future storage.Future[*OpenResponse]
}

// Seek 结构体携带了 os.File.Seek 需要的参数
type Seek struct {
	Offset int64
	Whence int
}

type ReadRequest struct {
	Buf []byte

	// 读取前先 Seek 到指定位置
	Seek *Seek
	// 是否将缓冲区读满
	Full bool
	// 文件读取完成的通知
	Future storage.Future[*ReadResponse]
}

type WriteRequest struct {
	Data []byte

	// 写入前先 Seek 到指定位置
	Seek *Seek
	// 写入后是否更新文件信息(os.FileInfo)
	GetInfo bool

	// 请求生命周期携带的上下文数据
	Context any

	// 文化写入完成的通知
	Future storage.Future[*WriteResponse]
}

type CloseRequest struct {
	// 请求生命周期携带的上下文数据
	Context any
	// 文化关闭完成的通知
	Future storage.Future[*CloseResponse]
}

type RemoveRequest struct {
	File string
	// 请求生命周期携带的上下文数据
	Context any
	// 文化删除完成的通知
	Future storage.Future[*RemoveResponse]
}
