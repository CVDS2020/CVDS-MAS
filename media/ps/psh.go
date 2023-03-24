package ps

import "io"

var PackHeaderStuffing = [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

type PSH struct {
	startCode [4]byte
	bytes10   [10]byte
}

func (h *PSH) StartCode() [4]byte {
	return h.startCode
}

func (h *PSH) MPEGFlag() uint8 {
	return (h.bytes10[0] & 0b11000000) >> 6
}

func (h *PSH) SystemClockReferenceBase() uint64 {
	return (uint64(h.bytes10[0]&0b00111000) << 27) |
		(uint64(h.bytes10[0]&0b00000011) << 28) |
		(uint64(h.bytes10[1]) << 20) |
		(uint64(h.bytes10[2]&0b11111000) << 12) |
		(uint64(h.bytes10[2]&0b00000011) << 13) |
		(uint64(h.bytes10[3]) << 5) |
		(uint64(h.bytes10[4]) >> 3)
}

func (h *PSH) SystemClockReferenceExtension() uint16 {
	return (uint16(h.bytes10[4]&0b00000011) << 7) | (uint16(h.bytes10[5]&0b11111110) >> 1)
}

func (h *PSH) ProgramMuxRate() uint32 {
	return (uint32(h.bytes10[6]) << 14) | (uint32(h.bytes10[7] << 6)) | (uint32(h.bytes10[8]) >> 2)
}

func (h *PSH) Reserved() uint8 {
	return h.bytes10[9] >> 3
}

func (h *PSH) PackStuffingLength() uint8 {
	return h.bytes10[9] & 0b00000111
}

func (h *PSH) Size() uint {
	if h.startCode[3] == 0 {
		return 0
	}
	return 14 + uint(h.PackStuffingLength())
}

func (h *PSH) WriteTo(w io.Writer) (n int64, err error) {
	if h.startCode[3] == 0 {
		return 0, nil
	}
	buf := [24]byte{}
	writer := Writer{Buf: buf[:]}
	err = WriteAndReset(writer.WriteBytesAnd(h.startCode[:]).
		WriteBytesAnd(h.bytes10[:]).
		WriteBytesAnd(PackHeaderStuffing[:h.PackStuffingLength()]), w, &n)
	return
}

func (h *PSH) Clear() {
	h.startCode = [4]byte{}
	h.bytes10[9] &= 0b11111000
}
