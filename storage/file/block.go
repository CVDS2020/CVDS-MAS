package file

import (
	"fmt"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/storage"
	"io"
	"sync/atomic"
	"time"
)

type Block struct {
	Frames      []storage.Frame
	size        uint
	Start       time.Time
	End         time.Time
	MediaType   uint32
	StorageType uint32
	IndexState  uint64
	ref         atomic.Int64
}

func (b *Block) Len() int {
	return len(b.Frames)
}

func (b *Block) Size() uint {
	return b.size
}

func (b *Block) Duration() time.Duration {
	return b.End.Sub(b.Start)
}

func timeRangeExpandRange(start, end *time.Time, curStart, curEnd time.Time, timeNodeLen int) {
	if start.After(*end) {
		Logger().Fatal("开始时间在结束时间之后", log.Time("开始时间", *start), log.Time("结束时间", *end))
	}
	if curStart.After(curEnd) {
		Logger().Fatal("开始时间在结束时间之后", log.Time("开始时间", curStart), log.Time("结束时间", curEnd))
	}
	if start.IsZero() {
		*start = curStart
		*end = curEnd
	} else {
		du := curStart.Sub(*end)
		if du < 0 {
			guess := time.Duration(0)
			if timeNodeLen > 2 {
				guess = (end.Sub(*start)) / time.Duration(timeNodeLen-2)
			} else if timeNodeLen == 2 {
				guess = curEnd.Sub(curStart)
			}
			offset := du - guess
			*start = start.Add(offset)
		}
		*end = curEnd
	}
}

func (b *Block) Append(frame storage.Frame) {
	if len(b.Frames) == 0 {
		b.MediaType = frame.MediaType()
		b.StorageType = frame.StorageType()
	}
	b.Frames = append(b.Frames, frame)
	b.size += frame.Size()
	timeRangeExpandRange(&b.Start, &b.End, frame.StartTime(), frame.EndTime(), len(b.Frames))
}

func (b *Block) WriteTo(w io.Writer) (n int64, err error) {
	for _, frame := range b.Frames {
		if err = storage.WriteTo(frame, w, &n); err != nil {
			return
		}
	}
	return
}

func (b *Block) Read(p []byte) (n int, err error) {
	w := storage.Writer{Buf: p}
	n64, _ := b.WriteTo(&w)
	return int(n64), io.EOF
}

func (b *Block) Free() {
	for _, frame := range b.Frames {
		frame.Release()
	}
	b.Frames = b.Frames[:0]
	b.size = 0
	b.Start, b.End = time.Time{}, time.Time{}
	b.MediaType, b.StorageType = 0, 0
}

func (b *Block) Release() {
	if c := b.ref.Add(-1); c == 0 {
		b.Free()
	} else if c < 0 {
		Logger().Fatal(fmt.Sprintf("Block 重复释放，引用计数值(%d -> %d)", c+1, c))
	}
}

func (b *Block) AddRef() {
	if c := b.ref.Add(1); c <= 0 {
		Logger().Fatal(fmt.Sprintf("错误的 Block 引用计数值(%d -> %d)", c-1, c))
	}
}

func (b *Block) Use() *Block {
	b.AddRef()
	return b
}

type DroppedBlock struct {
	Dropped uint
	size    uint
	Start   time.Time
	End     time.Time
}

func (b *DroppedBlock) Size() uint {
	return b.size
}

func (b *DroppedBlock) Duration() time.Duration {
	return b.End.Sub(b.Start)
}

func (b *DroppedBlock) Add(start, end time.Time, size uint) {
	b.Dropped++
	b.size += size
	timeRangeExpandRange(&b.Start, &b.End, start, end, int(b.Dropped))
}

func (b *DroppedBlock) Append(frame storage.Frame) {
	b.Add(frame.StartTime(), frame.EndTime(), frame.Size())
	frame.Release()
}
