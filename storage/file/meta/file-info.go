package meta

import (
	"encoding/binary"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/uns"
	"gitee.com/sy_183/cvds-mas/storage"
	"io"
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

type FileInfo struct {
	Seq       uint64 `gorm:"column:seq;primaryKey;not null;autoIncrement;comment:文件序列号"`
	Name      string `gorm:"column:name;not null;comment:文件名称，不包含路径前缀"`
	Path      string `gorm:"column:path;not null;comment:文件路径"`
	Size      uint64 `gorm:"column:size;not null;default:0;comment:文件大小，单位：byte"`
	StartTime int64  `gorm:"column:start_time;not null;comment:文件开始时间"`
	EndTime   int64  `gorm:"column:end_time;not null;comment:文件结束时间"`
	State     uint64 `gorm:"column:state;not null;comment:文件状态"`
}

func (f *FileInfo) Duration() time.Duration {
	return time.Duration(f.EndTime-f.StartTime) * time.Millisecond
}

func (f *FileInfo) Clone() *FileInfo {
	n := new(FileInfo)
	*n = *f
	return n
}

func (f *FileInfo) InfoSize() int {
	return 7*8 + len(f.Name) + len(f.Path)
}

func (f *FileInfo) Parse([]byte) {

}

func (f *FileInfo) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 8)
	defer func() {
		if e := recover(); e != nil {
			err, _ = e.(error)
		}
	}()
	binary.BigEndian.PutUint64(buf, f.Seq)
	assert.MustSuccess(storage.Write(w, buf, &n))
	binary.BigEndian.PutUint64(buf, uint64(len(f.Name)))
	assert.MustSuccess(storage.Write(w, buf, &n))
	assert.MustSuccess(storage.Write(w, uns.StringToBytes(f.Name), &n))
	binary.BigEndian.PutUint64(buf, uint64(len(f.Path)))
	assert.MustSuccess(storage.Write(w, buf, &n))
	assert.MustSuccess(storage.Write(w, uns.StringToBytes(f.Path), &n))
	binary.BigEndian.PutUint64(buf, f.Size)
	assert.MustSuccess(storage.Write(w, buf, &n))
	binary.BigEndian.PutUint64(buf, uint64(f.StartTime))
	assert.MustSuccess(storage.Write(w, buf, &n))
	binary.BigEndian.PutUint64(buf, uint64(f.EndTime))
	assert.MustSuccess(storage.Write(w, buf, &n))
	binary.BigEndian.PutUint64(buf, f.State)
	assert.MustSuccess(storage.Write(w, buf, &n))
	return
}

func (f *FileInfo) MarshalLogObject(encoder log.ObjectEncoder) error {
	encoder.AddUint64("seq", f.Seq)
	encoder.AddString("name", f.Name)
	encoder.AddString("path", f.Path)
	encoder.AddTime("start_time", time.UnixMilli(f.StartTime))
	encoder.AddTime("end_time", time.UnixMilli(f.EndTime))
	encoder.AddString("state", FileStateString(f.State))
	return nil
}
