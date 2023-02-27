package allocator

import _ "unsafe"

//go:linkname goPanicIndex runtime.goPanicIndex
func goPanicIndex(x int, y int)

type BitMap struct {
	ms  [][]byte
	cap uint64
}

func NewBitMap(cap uint64) *BitMap {
	bm := &BitMap{cap: cap}
	for cap > 1 || (cap > 0 && len(bm.ms) == 0) {
		m := make([]byte, (cap+7)>>3)
		if cap&7 > 0 {
			m[len(m)-1] = 0xff << (cap & 7)
		}
		bm.ms = append([][]byte{m}, bm.ms...)
		cap = uint64(len(m))
	}
	return bm
}

func (*BitMap) byteFreed(b byte) int64 {
	var k int64
	if b < 0xff {
		if b&0b00001111 < 0b00001111 {
			b = b & 0b00001111
		} else {
			b = (b & 0b11110000) >> 4
			k += 4
		}
		if b&0b0011 < 0b0011 {
			b = b & 0b0011
		} else {
			b = (b & 0b1100) >> 2
			k += 2
		}
		if b&0b01 == 0b01 {
			k += 1
		}
		return k
	}
	return -1
}

func (bm *BitMap) Alloc() int64 {
	var i, k int64
	for _, m := range bm.ms {
		if k = bm.byteFreed(m[i]); k < 0 {
			return -1
		}
		i = i<<3 + k
	}
	for j, i := len(bm.ms)-1, i; j >= 0; j-- {
		m, k := bm.ms[j], i&7
		i = i >> 3
		if m[i] |= 1 << k; m[i] < 0xff {
			break
		}
	}
	return i
}

func (bm *BitMap) AllocIndex(i int64) (allocated bool) {
	if i < 0 || i >= int64(bm.cap) {
		goPanicIndex(int(i), int(bm.cap))
	}
	for j, i := len(bm.ms)-1, i; j >= 0; j-- {
		m, k := bm.ms[j], i&7
		i = i >> 3
		b := m[i]
		m[i] = b | (1 << k)
		if j == len(bm.ms)-1 {
			allocated = b != m[i]
		}
		if m[i] < 0xff {
			break
		}
	}
	return
}

func (bm *BitMap) AllocAll() {
	for _, m := range bm.ms {
		for i := range m {
			m[i] = 0xff
		}
	}
}

func (bm *BitMap) Free(i int64) (freed bool) {
	if i < 0 || i >= int64(bm.cap) {
		goPanicIndex(int(i), int(bm.cap))
	}
	for j := len(bm.ms) - 1; j >= 0; j-- {
		m, k := bm.ms[j], i&7
		i = i >> 3
		b := m[i]
		m[i] = b & ^(1 << k)
		if j == len(bm.ms)-1 {
			freed = b != m[i]
		}
		if b < 0xff {
			break
		}
	}
	return
}
