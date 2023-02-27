package storage

type Index interface {
	Sequence() uint64

	StartTime() int64

	EndTime() int64

	DataSize() uint64

	GetState() uint64

	CloneIndex() Index
}

type Indexes interface {
	Len() int

	Cap() int

	Get(i int) Index

	Cut(start, end int) Indexes
}
