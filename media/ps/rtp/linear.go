package rtp

import (
	"gitee.com/sy_183/common/slice"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"io"
)

type Linear interface {
	Cap() int

	Size() int

	Bytes() []byte

	Slice(start, end int) []byte

	Cut(start, end int) Linear

	Range(f func(bs []byte) bool) bool

	io.WriterTo
}

type ByteLinear struct {
	bytes []byte
	start int
	end   int
}

func NewByteLinear(bs []byte) ByteLinear {
	return ByteLinear{bytes: bs, end: len(bs)}
}

func (l ByteLinear) Cap() int {
	return len(l.bytes)
}

func (l ByteLinear) Size() int {
	return l.end - l.start
}

func (l ByteLinear) Bytes() []byte {
	return l.bytes[l.start:l.end]
}

func (l ByteLinear) Slice(start, end int) []byte {
	return l.Cut(start, end).Bytes()
}

func (l ByteLinear) Cut(start, end int) Linear {
	switch {
	case start >= 0 && end >= 0:
		if start > end {
			slice.GoPanicSliceB(start, end)
		}
		s, e := l.start+start, l.end+end
		if e > l.end {
			slice.GoPanicSliceB(end, l.Size())
		}
		l.start = s
		l.end = e
		return l
	case start < 0 && end < 0:
		return l
	case start < 0:
		e := l.start + end
		if e > l.end {
			slice.GoPanicSliceB(end, l.Size())
		}
		l.end = e
		return l
	default:
		s := l.start + start
		if s > l.end {
			slice.GoPanicSliceB(start, l.Size())
		}
		l.start = s
		return l
	}
}

func (l ByteLinear) Range(f func(bs []byte) bool) bool {
	return f(l.bytes[l.start:l.end])
}

func (l ByteLinear) WriteTo(w io.Writer) (n int64, err error) {
	err = ioUtils.Write(w, l.bytes[l.start:l.end], &n)
	return
}

type ChunksLinear struct {
	chunks [][]byte
	cap    int
	start  int
	sx, sy int
	end    int
	ex, ey int
}

func NewChunksLinear(chunks [][]byte, size int) *ChunksLinear {
	if size < 0 {
		size = 0
		for _, chunk := range chunks {
			size += len(chunk)
		}
	}
	cl := &ChunksLinear{chunks: chunks, cap: size, end: size}
	if len(chunks) != 0 {
		cl.ex = len(chunks) - 1
		cl.ey = len(chunks[cl.ex])
	}
	return cl
}

func (l *ChunksLinear) Cap() int {
	return l.cap
}

func (l *ChunksLinear) Size() int {
	return l.end - l.start
}

func (l *ChunksLinear) Bytes() (bs []byte) {
	switch {
	case len(l.chunks) == 0 || l.sx > l.ex:
		return nil
	case l.sx == l.ex:
		return l.chunks[l.sx][l.sy:l.ey]
	case l.sx == l.ex-1:
		return slice.Join11(l.chunks[l.sx][l.sy:], l.chunks[l.ex][:l.ey])
	default:
		return slice.Join1N1(l.chunks[l.sx][l.sy:], l.chunks[l.sx+1:l.ex], l.chunks[l.ex][:l.ey])
	}
}

func (l *ChunksLinear) Slice(start, end int) []byte {
	nl := ChunksLinear{}
	l.CutTo(start, end, &nl)
	return nl.Bytes()
}

func (l *ChunksLinear) Cut(start, end int) Linear {
	nl := new(ChunksLinear)
	l.CutTo(start, end, nl)
	return nl
}

func (l *ChunksLinear) CutTo(start, end int, linear *ChunksLinear) {
	if l != linear {
		*linear = *l
	}
	switch {
	case start >= 0 && end >= 0:
		if start > end {
			slice.GoPanicSliceB(start, end)
		}
		s, e := linear.start+start, linear.end+end
		if e > linear.end {
			slice.GoPanicSliceB(end, linear.Size())
		}
		linear.sx, linear.sy = slice.LocateStart(linear.chunks, s)
		linear.ex, linear.ey = slice.LocateEnd(linear.chunks, e)
		linear.start = s
		linear.end = e
		return
	case start < 0 && end < 0:
		return
	case start < 0:
		e := linear.start + end
		if e > linear.end {
			slice.GoPanicSliceB(end, linear.Size())
		}
		linear.ex, linear.ey = slice.LocateEnd(linear.chunks, e)
		linear.end = e
		return
	default:
		s := linear.start + start
		if s > linear.end {
			slice.GoPanicSliceB(start, linear.Size())
		}
		linear.sx, linear.sy = slice.LocateStart(linear.chunks, s)
		linear.start = s
		return
	}
}

func (l *ChunksLinear) Range(f func(chunk []byte) bool) bool {
	switch {
	case len(l.chunks) == 0 || l.sx > l.ex:
		return true
	case l.sx == l.ex:
		return f(l.chunks[l.sx][l.sy:l.ey])
	default:
		if !f(l.chunks[l.sx][l.sy:]) {
			return false
		}
		for _, chunk := range l.chunks[l.sx+1 : l.ex] {
			if !f(chunk) {
				return false
			}
		}
		return f(l.chunks[l.ex][:l.ey])
	}
}

func (l *ChunksLinear) WriteTo(w io.Writer) (n int64, err error) {
	defer func() { err = ioUtils.HandleRecovery(recover()) }()
	l.Range(func(chunk []byte) bool {
		ioUtils.WritePanic(w, chunk, &n)
		return true
	})
	return
}

type LinearList struct {
	list   []Linear
	cap    int
	start  int
	sx, sy int
	end    int
	ex, ey int
}

func (l *LinearList) Cap() int {
	return l.cap
}

func (l *LinearList) Size() int {
	return l.end - l.start
}

func (l *LinearList) Bytes() (bs []byte) {
	direct := true
	l.Range(func(chunk []byte) bool {
		if len(chunk) > 0 {
			if direct {
				if bs == nil {
					bs = chunk
				} else {
					direct = false
					old := bs
					bs = make([]byte, 0, l.Size())
					bs = append(append(bs, old...), chunk...)
				}
			} else {
				bs = append(bs, chunk...)
			}
		}
		return true
	})
	return
}

func (l *LinearList) Slice(start, end int) []byte {
	nl := LinearList{}
	l.CutTo(start, end, &nl)
	return nl.Bytes()
}

func (l *LinearList) Cut(start, end int) Linear {
	nl := new(LinearList)
	l.CutTo(start, end, nl)
	return nl
}

func (l *LinearList) locateStart(start int) (sx, sy int) {
	if len(l.list) == 0 {
		return 0, 0
	}
	i := start
	for j, linear := range l.list {
		if i < linear.Size() {
			return j, i
		}
		i -= linear.Size()
	}
	if i > 0 {
		slice.GoPanicSliceAlen(start, start-i)
	}
	return len(l.list) - 1, l.list[len(l.list)-1].Size()
}

func (l *LinearList) locateEnd(end int) (ex, ey int) {
	if len(l.list) == 0 {
		return 0, 0
	}
	i := end
	for j, linear := range l.list {
		if i <= linear.Size() {
			return j, i
		}
		i -= linear.Size()
	}
	slice.GoPanicSliceAlen(end, end-i)
	return 0, 0
}

func (l *LinearList) CutTo(start, end int, linear *LinearList) {
	if l != linear {
		*linear = *l
	}
	switch {
	case start >= 0 && end >= 0:
		if start > end {
			slice.GoPanicSliceB(start, end)
		}
		s, e := linear.start+start, linear.end+end
		if e > linear.end {
			slice.GoPanicSliceB(end, linear.Size())
		}
		linear.sx, linear.sy = l.locateStart(s)
		linear.ex, linear.ey = l.locateEnd(e)
		linear.start = s
		linear.end = e
		return
	case start < 0 && end < 0:
		return
	case start < 0:
		e := linear.start + end
		if e > linear.end {
			slice.GoPanicSliceB(end, linear.Size())
		}
		linear.ex, linear.ey = l.locateEnd(e)
		linear.end = e
		return
	default:
		s := linear.start + start
		if s > linear.end {
			slice.GoPanicSliceB(start, linear.Size())
		}
		linear.sx, linear.sy = l.locateStart(s)
		linear.start = s
		return
	}
}

func (l *LinearList) Append(linear Linear) {
	oldLen := len(l.list)
	oldCap := l.cap
	l.list = append(l.list, linear)
	l.cap += linear.Size()
	if l.end == oldCap {
		l.end = l.cap
		l.ex = len(l.list) - 1
		l.ey = l.list[l.ex].Size()
	}
	if oldLen > 0 && l.sx == oldLen-1 && l.sy == l.list[oldLen-1].Size() {
		l.sx = oldLen
		l.sy = 0
	}
}

func (l *LinearList) Range(f func(bs []byte) bool) bool {
	switch {
	case len(l.list) == 0 || l.sx > l.ex:
		return true
	case l.sx == l.ex:
		return l.list[l.sx].Cut(l.sy, l.ey).Range(f)
	default:
		if !l.list[l.sx].Cut(l.sy, -1).Range(f) {
			return false
		}
		for _, chunk := range l.list[l.sx+1 : l.ex] {
			if chunk.Range(f) {
				return false
			}
		}
		return l.list[l.ex].Cut(-1, l.ey).Range(f)
	}
}

func (l *LinearList) WriteTo(w io.Writer) (n int64, err error) {
	defer func() { err = ioUtils.HandleRecovery(recover()) }()
	l.Range(func(chunk []byte) bool {
		ioUtils.WritePanic(w, chunk, &n)
		return true
	})
	return
}
