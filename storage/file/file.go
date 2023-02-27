package file

import (
	"gitee.com/sy_183/common/log"
	"time"
)

const (
	FileStateWriting = 0 << 0
	FileStateWrote   = 1 << 0

	FileStateExist    = 0 << 2
	FileStateNotExist = 1 << 2
)

func FileHandleStateString(state uint64) string {
	switch state & (0b11 << 0) {
	case FileStateWriting:
		return "FILE_WRITING"
	case FileStateWrote:
		return "FILE_WROTE"
	default:
		return "FILE_HANDEL_UNKNOWN"
	}
}

func FileExistStateString(state uint64) string {
	switch state & (0b1 << 2) {
	case FileStateExist:
		return "FILE_EXIST"
	case FileStateNotExist:
		return "FILE_NOT_EXIST"
	default:
		return "FILE_EXIST_UNKNOWN"
	}
}

func FileStateString(state uint64) string {
	return FileHandleStateString(state) + "|" + FileExistStateString(state)
}

type File struct {
	Seq         uint64 `gorm:"column:seq;primaryKey;not null;autoIncrement;comment:文件序列号"`
	Name        string `gorm:"column:name;not null;comment:文件名称，不包含路径前缀"`
	Path        string `gorm:"column:path;not null;comment:文件路径"`
	Size        uint64 `gorm:"column:size;not null;default:0;comment:文件大小，单位：byte"`
	CreateTime  int64  `gorm:"column:create_time;not null;comment:文件创建时间"`
	StartTime   int64  `gorm:"column:start_time;not null;comment:文件开始时间"`
	EndTime     int64  `gorm:"column:end_time;not null;comment:文件结束时间"`
	MediaType   uint32 `gorm:"column:media_type;not null;comment:媒体类型"`
	StorageType uint32 `gorm:"column:storage_type;not null;comment:存储类型"`
	State       uint64 `gorm:"column:state;not null;comment:文件状态"`
}

func (*File) fileInterface() {}

func (f *File) Duration() time.Duration {
	return time.Duration(f.EndTime-f.StartTime) * time.Millisecond
}

func (f *File) Clone() *File {
	n := new(File)
	*n = *f
	return n
}

func (f *File) MarshalLogObject(encoder log.ObjectEncoder) error {
	encoder.AddUint64("seq", f.Seq)
	encoder.AddString("name", f.Name)
	encoder.AddString("path", f.Path)
	encoder.AddUint64("size", f.Size)
	encoder.AddTime("create_time", time.UnixMilli(f.CreateTime))
	encoder.AddTime("start_time", time.UnixMilli(f.StartTime))
	encoder.AddTime("end_time", time.UnixMilli(f.EndTime))
	encoder.AddUint32("media_time", f.MediaType)
	encoder.AddUint32("storage_type", f.StorageType)
	encoder.AddString("state", FileStateString(f.State))
	return nil
}
