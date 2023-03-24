package storage

import (
	"gitee.com/sy_183/common/log"
)

type Index interface {
	Seq() uint64

	Start() int64

	SetStart(start int64) Index

	End() int64

	SetEnd(end int64) Index

	FileSeq() uint64

	SetFileSeq(seq uint64) Index

	FileOffset() uint64

	SetFileOffset(offset uint64) Index

	Size() uint64

	SetSize(size uint64) Index

	StorageType() uint32

	SetStorageType(typ uint32) Index

	State() uint64

	SetState(state uint64) Index

	Clone() Index

	log.ObjectMarshaler
}

type Indexes interface {
	Len() int

	Cap() int

	Get(i int) Index

	Cut(start, end int) Indexes
}
