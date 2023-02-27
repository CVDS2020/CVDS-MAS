package meta

import (
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/slice"
	"gitee.com/sy_183/cvds-mas/storage"
	"io"
	"math"
	"os"
	"unsafe"
)

const IndexBlockFileMagic = 0x0f110e25

type IndexBlockFileHeader struct {
	Magic     uint32
	BlockSize uint32
}

var indexBlockFileHeaderSize = int(unsafe.Sizeof(IndexBlockFileHeader{}))

type IndexBlockHeader struct {
	Seq   uint64
	Start int64
	End   int64
	Len   uint64
}

var indexBlockHeaderSize = int(unsafe.Sizeof(IndexBlockHeader{}))

var (
	pageSize          = os.Getpagesize()
	minPageSize       = indexBlockHeaderSize + indexSize
	MinIndexBlockSize = uint32(pageSize)
	MaxIndexBlockSize = uint32(math.MaxInt32)
)

func init() {
	if pageSize < minPageSize {
		Logger().Fatal("page size is too small", log.Int("page size", pageSize), log.Int("min page size", minPageSize))
	}
}

type IndexBlock struct {
	raw []byte
	*IndexBlockHeader
	indexes []Index
}

func NewIndexBlock(raw []byte) *IndexBlock {
	if raw == nil {
		return new(IndexBlock)
	}
	return new(IndexBlock).Reset(raw)
}

func NewIndexBlockWith(data []byte, check bool) (*IndexBlock, error) {
	b := new(IndexBlock)
	if err := b.ResetWith(data, check); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *IndexBlock) checkRaw() error {
	if len(b.raw) < int(MinIndexBlockSize) || len(b.raw) > int(MaxIndexBlockSize) {
		return &IndexBlockBlockSizeError{BlockSize: uint32(len(b.raw))}
	}
	return nil
}

func (b *IndexBlock) check(check bool) error {
	if b.Seq == 0 {
		return nil
	}
	if b.Len > uint64(cap(b.indexes)) {
		return &IndexBlockLenError{
			IndexBlockInfo: IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
			Len:            b.Len,
			Cap:            uint64(cap(b.indexes)),
		}
	}
	b.indexes = b.indexes[:b.Len]
	if check {
		if b.Start > b.End {
			return &IndexBlockTimestampRangeError{
				TimestampRangeError: TimestampRangeError{Start: b.Start, End: b.End},
				IndexBlockInfo:      IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
			}
		}
		var last *Index
		for i := 0; i < len(b.indexes); i++ {
			index := &b.indexes[i]
			if index.Start > index.End {
				return &IndexBlockIndexTimestampRangeError{
					TimestampRangeError: TimestampRangeError{Start: index.Start, End: index.End},
					IndexBlockInfo:      IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
					IndexInfo:           IndexInfo{Seq: int64(index.Seq), Loc: i},
				}
			}
			if last != nil {
				if index.Seq <= last.Seq {
					return &IndexBlockAdjacentIndexSeqError{
						IndexBlockInfo: IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
						LastIndexInfo:  IndexInfo{Seq: int64(last.Seq), Loc: i - 1},
						IndexInfo:      IndexInfo{Seq: int64(index.Seq), Loc: i},
					}
				}
				if index.Start < last.End {
					return &IndexBlockAdjacentIndexTimestampError{
						IndexBlockInfo: IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
						LastEnd:        last.End,
						Start:          index.Start,
						LastIndexInfo:  IndexInfo{Seq: int64(last.Seq), Loc: i - 1},
						IndexInfo:      IndexInfo{Seq: int64(index.Seq), Loc: i},
					}
				}
			}
			last = index
		}
	}
	return nil
}

func (b *IndexBlock) Reset(raw []byte) *IndexBlock {
	b.raw = raw
	assert.MustSuccess(b.checkRaw())
	b.IndexBlockHeader = (*IndexBlockHeader)(unsafe.Pointer(&raw[0]))
	b.indexes = slice.Make[Index](unsafe.Pointer(&raw[indexBlockHeaderSize]), 0, (indexBlockHeaderSize-len(raw))/indexSize)
	return b
}

func (b *IndexBlock) ResetWith(data []byte, check bool) error {
	if data != nil {
		b.Reset(data)
	}
	return b.check(check)
}

func (b *IndexBlock) ReadFrom(r io.Reader) (n int64, err error) {
	assert.MustSuccess(b.checkRaw())
	var i int
	i, err = io.ReadFull(r, b.raw)
	n = int64(i)
	if err != nil {
		return
	}
	return n, b.check(false)
}

func (b *IndexBlock) Cap() int {
	return cap(b.indexes)
}

func (b *IndexBlock) Full() bool {
	return len(b.indexes) == cap(b.indexes)
}

func (b *IndexBlock) Raw() []byte {
	return b.raw
}

func (b *IndexBlock) WriteTo(w io.Writer) (n int64, err error) {
	err = storage.Write(w, b.raw, &n)
	return
}

func (b *IndexBlock) Append(index *Index, check bool) {
	if len(b.indexes) == cap(b.indexes) {
		panic(&IndexBlockFullError{
			IndexBlockInfo: IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
			Cap:            uint64(cap(b.indexes)),
		})
	}
	if check {
		if index.Start > index.End {
			panic(&IndexBlockIndexTimestampRangeError{
				TimestampRangeError: TimestampRangeError{Start: index.Start, End: index.End},
				IndexBlockInfo:      IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
				IndexInfo:           IndexInfo{Seq: int64(index.Seq), Loc: -1},
			})
		}
		if b.Len > 0 {
			last := &b.indexes[b.Len-1]
			if index.Seq <= last.Seq {
				panic(&IndexBlockAdjacentIndexSeqError{
					IndexBlockInfo: IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
					LastIndexInfo:  IndexInfo{Seq: int64(last.Seq), Loc: int(b.Len - 1)},
					IndexInfo:      IndexInfo{Seq: int64(index.Seq), Loc: -1},
				})
			}
			if index.Start < last.End {
				panic(&IndexBlockAdjacentIndexTimestampError{
					IndexBlockInfo: IndexBlockInfo{Seq: int64(b.Seq), Loc: -1},
					LastEnd:        last.End,
					Start:          index.Start,
					LastIndexInfo:  IndexInfo{Seq: int64(last.Seq), Loc: int(b.Len - 1)},
					IndexInfo:      IndexInfo{Seq: int64(index.Seq), Loc: -1},
				})
			}
		}
	}
	b.indexes = b.indexes[:b.Len+1]
	b.indexes[b.Len] = *index
	if b.Len == 0 {
		b.Start = index.Start
	}
	b.End = index.End
	b.Len++
}

func (b *IndexBlock) Get(i int) Index {
	return b.indexes[i]
}
