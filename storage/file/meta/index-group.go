package meta

import (
	"gitee.com/sy_183/common/uns"
	"unsafe"
)

const (
	IndexGroupStateNotBlock        = 0 << 0
	IndexGroupStateHasBlockData    = 1 << 0
	IndexGroupStateHasBlockNotData = 2 << 0
	IndexGroupStateHasBlockDropped = 3 << 0
	IndexGroupStateHasBlockIOError = 4 << 0

	IndexGroupStateNotStream        = 0 << 3
	IndexGroupStateHasStream        = 1 << 3
	IndexGroupStateHasStreamStopped = 2 << 3
	IndexGroupStateHasStreamLost    = 3 << 3
)

func IndexGroupBlockStateString(state uint64) string {
	switch state & (0b111 << 0) {
	case IndexGroupStateNotBlock:
		return "NOT_BLOCK"
	case IndexGroupStateHasBlockData:
		return "HAS_BLOCK_DATA"
	case IndexGroupStateHasBlockNotData:
		return "HAS_BLOCK_NOT_DATA"
	case IndexGroupStateHasBlockDropped:
		return "HAS_BLOCK_DROPPED"
	case IndexGroupStateHasBlockIOError:
		return "HAS_BLOCK_IO_ERROR"
	default:
		return "BLOCK_UNKNOWN"
	}
}

func IndexGroupStreamString(state uint64) string {
	switch state & (0b11 << 3) {
	case IndexGroupStateNotStream:
		return "NOT_STREAM"
	case IndexGroupStateHasStream:
		return "HAS_STREAM"
	case IndexGroupStateHasStreamStopped:
		return "HAS_STREAM_STOPPED"
	case IndexGroupStateHasStreamLost:
		return "HAS_STREAM_LOST"
	default:
		return "STREAM_UNKNOWN"
	}
}

func IndexGroupStateSting(state uint64) string {
	return IndexGroupBlockStateString(state) + "|" + IndexGroupStreamString(state)
}

type IndexGroup struct {
	Seq   uint64 `gorm:"column:seq;primaryKey;not null;autoIncrement;comment:索引组序列号，使用文件存储时为索引组对应的所有索引存储的文件名，使用数据库存储时为表的主键"`
	Start int64  `gorm:"column:start;not null;comment:索引组开始时间(索引组第一个索引的开始时间)"`
	End   int64  `gorm:"column:end;not null;comment:索引组结束时间(索引组最后一个索引的结束时间)"`
	Len   uint64 `gorm:"column:offset;not null;comment:索引组包含的索引数量"`
	Size  uint64 `gorm:"column:size;not null;comment:索引组中所有索引数据块大小总和，单位：byte"`
	State uint64 `gorm:"column:state;not null;comment:索引组状态"`
}

var indexGroupSize = int(unsafe.Sizeof(IndexGroup{}))

func (ig *IndexGroup) Clone() *IndexGroup {
	nig := new(IndexGroup)
	*nig = *ig
	return nig
}

func (ig *IndexGroup) Bytes() []byte {
	return uns.MakeBytes(unsafe.Pointer(ig), indexGroupSize, indexGroupSize)
}
