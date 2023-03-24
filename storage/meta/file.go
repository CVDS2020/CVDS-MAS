package meta

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
	Seq uint64 `gorm:"column:seq;primaryKey;not null;autoIncrement;comment:文件序列号"`
	//Name       string `gorm:"column:name;not null;comment:文件名称，不包含路径前缀"`
	Path       string `gorm:"column:path;not null;comment:文件路径"`
	Size       uint64 `gorm:"column:size;not null;default:0;comment:文件大小，单位：byte"`
	CreateTime int64  `gorm:"column:createTime;not null;comment:文件创建时间"`
	StartTime  int64  `gorm:"column:startTime;not null;comment:文件开始时间"`
	EndTime    int64  `gorm:"column:endTime;not null;comment:文件结束时间"`
	State      uint64 `gorm:"column:state;not null;comment:文件状态"`
}

var FileModel = new(File)

func (f *File) Duration() time.Duration {
	return time.Duration(f.EndTime-f.StartTime) * time.Millisecond
}

func (f *File) Clone() *File {
	n := new(File)
	*n = *f
	return n
}

func (f *File) MarshalLogObject(encoder log.ObjectEncoder) error {
	encoder.AddUint64("文件序号", f.Seq)
	//encoder.AddString("文件名称", f.Name)
	encoder.AddString("文件路径", f.Path)
	encoder.AddUint64("文件大小", f.Size)
	encoder.AddTime("文件创建时间", time.UnixMilli(f.CreateTime))
	if f.StartTime > 0 {
		encoder.AddTime("文件起始时间", time.UnixMilli(f.StartTime))
		encoder.AddTime("文件结束时间", time.UnixMilli(f.EndTime))
	}
	encoder.AddString("文件状态", FileStateString(f.State))
	return nil
}
