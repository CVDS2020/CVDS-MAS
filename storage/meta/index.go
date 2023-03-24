package meta

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
	SeqV         uint64 `gorm:"column:seq;primaryKey;not null;autoIncrement;comment:索引序列号"`
	StartV       int64  `gorm:"column:start;not null;comment:数据块开始时间"`
	EndV         int64  `gorm:"column:end;not null;comment:数据块结束时间"`
	FileSeqV     uint64 `gorm:"column:fileSeq;not null;comment:文件序列号"`
	FileOffsetV  uint64 `gorm:"column:fileOffset;not null;comment:数据块在文件中的偏移量"`
	SizeV        uint64 `gorm:"column:size;not null;comment:数据块大小，单位：byte"`
	StorageTypeV uint32 `gorm:"column:storageType;not null;comment:存储类型"`
	StateV       uint64 `gorm:"column:state;not null;comment:索引状态"`
}

var IndexModel = new(Index)

func (i *Index) Seq() uint64 {
	return i.SeqV
}

func (i *Index) Start() int64 {
	return i.StartV
}

func (i *Index) SetStart(start int64) storage.Index {
	i.StartV = start
	return i
}

func (i *Index) End() int64 {
	return i.EndV
}

func (i *Index) SetEnd(end int64) storage.Index {
	i.EndV = end
	return i
}

func (i *Index) FileSeq() uint64 {
	return i.FileSeqV
}

func (i *Index) SetFileSeq(seq uint64) storage.Index {
	i.FileSeqV = seq
	return i
}

func (i *Index) FileOffset() uint64 {
	return i.FileOffsetV
}

func (i *Index) SetFileOffset(offset uint64) storage.Index {
	i.FileOffsetV = offset
	return i
}

func (i *Index) Size() uint64 {
	return i.SizeV
}

func (i *Index) SetSize(size uint64) storage.Index {
	i.SizeV = size
	return i
}

func (i *Index) StorageType() uint32 {
	return i.StorageTypeV
}

func (i *Index) SetStorageType(typ uint32) storage.Index {
	i.StorageTypeV = typ
	return i
}

func (i *Index) State() uint64 {
	return i.StateV
}

func (i *Index) SetState(state uint64) storage.Index {
	i.StateV = state
	return i
}

func (i *Index) clone() *Index {
	n := new(Index)
	*n = *i
	return n
}

func (i *Index) Clone() storage.Index {
	return i.clone()
}

func (i *Index) MarshalLogObject(encoder log.ObjectEncoder) error {
	encoder.AddUint64("索引序号", i.SeqV)
	encoder.AddTime("索引起始时间", time.UnixMilli(i.StartV).Local())
	encoder.AddTime("索引结束时间", time.UnixMilli(i.EndV).Local())
	if indexMask := i.StateV & (0b1 << 0); indexMask == IndexStateNormal {
		if blockMask := i.StateV & (0b11 << 1); blockMask != IndexStateBlockNotData {
			if blockMask == IndexStateBlockData {
				encoder.AddUint64("文件序号", i.FileSeqV)
				encoder.AddUint64("文件偏移", i.FileOffsetV)
			}
			encoder.AddUint64("块大小", i.SizeV)
			if typ := storage.GetStorageType(i.StorageTypeV); typ != nil {
				encoder.AddString("存储类型", typ.Name)
			} else {
				encoder.AddUint32("存储类型", i.StorageTypeV)
			}
		}
	}
	encoder.AddString("索引状态", IndexStateString(i.StateV))
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
