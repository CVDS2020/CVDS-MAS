package h264

import (
	ioUtils "gitee.com/sy_183/common/utils/io"
	"io"
)

const (
	NALUTypeSlice    = 1
	NALUTypeDPA      = 2
	NALUTypeDPB      = 3
	NALUTypeDBC      = 4
	NALUTypeIDR      = 5
	NALUTypeSEI      = 6
	NALUTypeSPS      = 7
	NALUTypePPS      = 8
	NALUTypeAUD      = 9
	NALUTypeEOSEQ    = 10
	NALUTypeEOSTRREM = 11
	NALUTypeFILL     = 12

	NALUTypeSTAPA  = 24
	NALUTypeSTAPB  = 25
	NALUTypeMTAP16 = 26
	NALUTypeMTAP24 = 27
	NALUTypeFUA    = 28
	NALUTypeFUB    = 28
)

type NALUHeader byte

func (h NALUHeader) Type() uint8 {
	return uint8(h & 0b00011111)
}

func (h *NALUHeader) SetType(typ uint8) {
	*h |= NALUHeader(typ & 0b00011111)
}

func (h NALUHeader) NRI() uint8 {
	return (uint8(h) >> 5) & 0b11
}

func (h *NALUHeader) SetNRI(nri uint8) {
	*h |= NALUHeader((nri & 0b11) << 5)
}

type FUHeader byte

func (h FUHeader) IsStart() bool {
	return h>>7 == 1
}

func (h *FUHeader) SetStart(set bool) {
	if set {
		*h |= 0b10000000
	} else {
		*h &= 0b01111111
	}
}

func (h FUHeader) IsEnd() bool {
	return (h>>6)&0b01 == 1
}

func (h *FUHeader) SetEnd(set bool) {
	if set {
		*h |= 0b01000000
	} else {
		*h &= 0b10111111
	}
}

func (h FUHeader) Type() uint8 {
	return uint8(h & 0b00011111)
}

func (h *FUHeader) SetType(typ uint8) {
	*h |= FUHeader(typ & 0b00011111)
}

type NALUBody struct {
	chunks [][]byte
	size   int
}

func (b *NALUBody) Append(chunk []byte) {
	b.chunks = append(b.chunks, chunk)
	b.size += len(chunk)
}

func (b *NALUBody) Range(f func(chunk []byte) bool) bool {
	for _, chunk := range b.chunks {
		if !f(chunk) {
			return false
		}
	}
	return true
}

func (b *NALUBody) Size() int {
	return b.size
}

func (b *NALUBody) Bytes() []byte {
	w := ioUtils.Writer{Buf: make([]byte, b.Size())}
	n, _ := b.WriteTo(&w)
	return w.Buf[:n]
}

func (b *NALUBody) Read(buf []byte) (n int, err error) {
	w := ioUtils.Writer{Buf: buf}
	_n, _ := b.WriteTo(&w)
	return int(_n), io.EOF
}

func (b *NALUBody) WriteTo(w io.Writer) (n int64, err error) {
	for _, content := range b.chunks {
		if err = ioUtils.Write(w, content, &n); err != nil {
			return
		}
	}
	return
}

func (b *NALUBody) Clear() {
	b.chunks = b.chunks[:0]
}
