package file

import (
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/storage"
	"time"
)

const (
	IndexStateNormal  = 0 << 0
	IndexStateDropped = 1 << 0

	IndexStateBlockData    = 0 << 1
	IndexStateBlockNotData = 1 << 1
	IndexStateBlockDropped = 2 << 1
	IndexStateBlockIOError = 3 << 1

	IndexStateStreamNormal  = 0 << 3
	IndexStateStreamStopped = 1 << 3
	IndexStateStreamLost    = 2 << 3
)

func IndexIndexStateString(state uint64) string {
	switch state & (0b1 << 0) {
	case IndexStateNormal:
		return "INDEX_NORMAL"
	case IndexStateDropped:
		return "INDEX_DROPPED"
	default:
		return "INDEX_UNKNOWN"
	}
}

func IndexBlockStateString(state uint64) string {
	switch state & (0b11 << 1) {
	case IndexStateBlockData:
		return "BLOCK_DATA"
	case IndexStateBlockNotData:
		return "BLOCK_NOT_DATA"
	case IndexStateBlockDropped:
		return "BLOCK_DROPPED"
	case IndexStateBlockIOError:
		return "BLOCK_IO_ERROR"
	default:
		return "BLOCK_UNKNOWN"
	}
}

func IndexStreamStateString(state uint64) string {
	switch state & (0b11 << 3) {
	case IndexStateStreamNormal:
		return "STREAM_NORMAL"
	case IndexStateStreamStopped:
		return "STREAM_STOPPED"
	case IndexStateStreamLost:
		return "STREAM_LOST"
	default:
		return "STREAM_UNKNOWN"
	}
}

func IndexStateString(state uint64) string {
	return IndexIndexStateString(state) + "|" + IndexBlockStateString(state) + "|" + IndexStreamStateString(state)
}

type Index struct {
	Seq        uint64 `gorm:"column:seq;primaryKey;not null;autoIncrement;comment:索引序列号"`
	Start      int64  `gorm:"column:start;not null;comment:数据块开始时间"`
	End        int64  `gorm:"column:end;not null;comment:数据块结束时间"`
	FileSeq    uint64 `gorm:"column:file_seq;not null;comment:文件序列号"`
	FileOffset uint64 `gorm:"column:file_offset;not null;comment:数据块在文件中的偏移量"`
	Size       uint64 `gorm:"column:size;not null;comment:数据块大小，单位：byte"`
	State      uint64 `gorm:"column:state;not null;comment:索引状态"`
}

func (i *Index) Sequence() uint64 {
	return i.Seq
}

func (i *Index) StartTime() int64 {
	return i.Start
}

func (i *Index) EndTime() int64 {
	return i.End
}

func (i *Index) DataSize() uint64 {
	return i.Size
}

func (i *Index) GetState() uint64 {
	return i.State
}

func (i *Index) Clone() *Index {
	n := new(Index)
	*n = *i
	return n
}

func (i *Index) CloneIndex() storage.Index {
	return i.Clone()
}

func (i *Index) MarshalLogObject(encoder log.ObjectEncoder) error {
	encoder.AddUint64("seq", i.Seq)
	encoder.AddTime("start", time.UnixMilli(i.Start).Local())
	encoder.AddTime("end", time.UnixMilli(i.End).Local())
	if i.State&(0b11<<1) == IndexStateBlockData {
		encoder.AddUint64("file_seq", i.FileSeq)
		encoder.AddUint64("file_offset", i.FileOffset)
		encoder.AddUint64("size", i.Size)
	}
	encoder.AddString("state", IndexStateString(i.State))
	return nil
}

type Indexes []Index

func (is Indexes) Len() int {
	return len(is)
}

func (is Indexes) Cap() int {
	return cap(is)
}

func (is Indexes) Get(i int) storage.Index {
	return &is[i]
}

func (is Indexes) Cut(start, end int) storage.Indexes {
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = len(is)
	}
	return is[start:end]
}
