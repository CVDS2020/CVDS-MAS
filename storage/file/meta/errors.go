package meta

import (
	"errors"
	"fmt"
)

type TimestampRangeError struct {
	Start int64
	End   int64
}

func (e *TimestampRangeError) Error() string {
	return fmt.Sprintf("invalid timestamp range, start timestamp[%d] is after end timestamp[%d]", e.Start, e.End)
}

func IndexSimpleName(name string, seq int64, loc int) string {
	switch {
	case seq < 0 && loc < 0:
		return name
	case seq >= 0 && loc < 0:
		return fmt.Sprintf("%s[seq:%d]", name, seq)
	case seq < 0 && loc >= 0:
		return fmt.Sprintf("%s[loc:%d]", name, loc)
	default:
		return fmt.Sprintf("%s[seq:%d loc:%d]", name, seq, loc)
	}
}

type IndexError interface {
	SetSeq(seq int64)
	SetLoc(loc int)
}

type IndexBlockError interface {
	IndexError
}

type IndexInfo struct {
	Seq int64
	Loc int
}

func (i *IndexInfo) SetSeq(seq int64) {
	i.Seq = seq
}

func (i *IndexInfo) SetLoc(loc int) {
	i.Loc = loc
}

func (i *IndexInfo) Name(name string) string {
	return IndexSimpleName(name, i.Seq, i.Loc)
}

type IndexBlockInfo IndexInfo

func (i *IndexBlockInfo) SetSeq(seq int64) {
	(*IndexInfo)(i).SetSeq(seq)
}

func (i *IndexBlockInfo) SetLoc(loc int) {
	(*IndexInfo)(i).SetLoc(loc)
}

func (i *IndexBlockInfo) Name(name string) string {
	return (*IndexInfo)(i).Name(name)
}

type IndexBlockFileMagicError struct {
	Magic uint32
}

func (e *IndexBlockFileMagicError) Error() string {
	return fmt.Sprintf("invalid index block file magic[0x%08x], must be [0x%08x]", e.Magic, IndexBlockFileMagic)
}

type IndexBlockBlockSizeError struct {
	BlockSize uint32
}

func (e *IndexBlockBlockSizeError) Error() string {
	if e.BlockSize < MinIndexBlockSize {
		return fmt.Sprintf("index block block size[%d] must greater than or equal to %d", e.BlockSize, MinIndexBlockSize)
	}
	if e.BlockSize > MaxIndexBlockSize {
		return fmt.Sprintf("index block block size[%d] must less than or equal to %d", e.BlockSize, MaxIndexBlockSize)
	}
	return ""
}

type IndexBlockSeqRepeatError struct {
	LastIndexBlockInfo IndexBlockInfo
	IndexBlockInfo     IndexBlockInfo
}

func (e *IndexBlockSeqRepeatError) Error() string {
	return fmt.Sprintf("invalid adjacent index block timestamp, %s sequence is equal to %s sequence",
		e.LastIndexBlockInfo.Name("last index block"),
		e.IndexBlockInfo.Name("current index block"),
	)
}

type AdjacentIndexBlockTimestampError struct {
	LastEnd            int64
	Start              int64
	LastIndexBlockInfo IndexBlockInfo
	IndexBlockInfo     IndexBlockInfo
}

func (e *AdjacentIndexBlockTimestampError) Error() string {
	return fmt.Sprintf("invalid adjacent index block timestamp, %s end timestamp[%d] is after %s start timestamp[%d]",
		e.LastIndexBlockInfo.Name("last index block"), e.LastEnd,
		e.IndexBlockInfo.Name("current index block"), e.Start,
	)
}

type IndexBlockLenError struct {
	IndexBlockInfo
	Cap uint64
	Len uint64
}

func (e *IndexBlockLenError) Error() string {
	return fmt.Sprintf("invalid %s length, length[%d] is larger than capacity[%d]",
		e.IndexBlockInfo.Name("index block"), e.Len, e.Cap,
	)
}

type IndexBlockTimestampRangeError struct {
	TimestampRangeError
	IndexBlockInfo
}

func (e *IndexBlockTimestampRangeError) Error() string {
	return fmt.Sprintf("invalid %s timestamp range, start timestamp[%d] is after end timestamp[%d]",
		e.IndexBlockInfo.Name("index block"), e.Start, e.End,
	)
}

type IndexBlockIndexTimestampRangeError struct {
	TimestampRangeError
	IndexBlockInfo
	IndexInfo IndexInfo
}

func (e *IndexBlockIndexTimestampRangeError) Error() string {
	return fmt.Sprintf("invalid %s %s timestamp range, start timestamp[%d] is after end timestamp[%d]",
		e.IndexBlockInfo.Name("index block"),
		e.IndexInfo.Name("index"), e.Start, e.End,
	)
}

type IndexBlockAdjacentIndexSeqError struct {
	IndexBlockInfo
	LastIndexInfo IndexInfo
	IndexInfo     IndexInfo
}

func (e *IndexBlockAdjacentIndexSeqError) Error() string {
	return fmt.Sprintf("invalid %s adjacent index sequence, %s sequence is greater than or equal to %s sequence",
		e.IndexBlockInfo.Name("index block"),
		e.LastIndexInfo.Name("last index"),
		e.IndexInfo.Name("current index"),
	)
}

type IndexBlockAdjacentIndexTimestampError struct {
	IndexBlockInfo
	LastEnd       int64
	Start         int64
	LastIndexInfo IndexInfo
	IndexInfo     IndexInfo
}

func (e *IndexBlockAdjacentIndexTimestampError) Error() string {
	return fmt.Sprintf("invalid %s adjacent index timestamp, %s end timestamp[%d] is after %s start timestamp[%d]",
		e.IndexBlockInfo.Name("index block"),
		e.LastIndexInfo.Name("last index"), e.LastEnd,
		e.IndexInfo.Name("current index"), e.Start,
	)
}

type IndexBlockDataSizeError struct {
	IndexBlockInfo
	Size    uint64
	SumSize uint64
}

func (e *IndexBlockDataSizeError) Error() string {
	return fmt.Sprintf("invalid %s data size, data size[%d] is not equal to the sum of all index data sizes[%d]",
		e.IndexBlockInfo.Name("index block"), e.Size, e.SumSize,
	)
}

type IndexBlockFullError struct {
	IndexBlockInfo
	Cap uint64
}

func (e *IndexBlockFullError) Error() string {
	return fmt.Sprintf("%s full, capacity[%d]", e.IndexBlockInfo.Name("index block"), e.Cap)
}

var FlushLatestIndexTaskFullError = errors.New("task of flush latest index is full")
