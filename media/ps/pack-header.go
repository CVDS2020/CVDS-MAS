package ps

import (
	"bytes"
	"encoding/binary"
)

const PackStartCode = 0x000001BA
const PackHeaderDisplay = "ps pack header"

var PackHeaderStuffing = [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

type PackHeader struct {
	packStartCode uint32
	bytes10       [10]byte
}

func (h *PackHeader) Clear() {
	h.packStartCode = 0
}

//func (h *PackHeader) Parse(s *stream.QueueStream) ParseResult {
//	need := uint(14)
//	data, err := s.Peek(need)
//	if err != nil {
//		return ParseResult{State: ParseStateNotEnough, Need: uint32(need)}
//	}
//	packStuffingLength := uint(data[13] & 0b00000111)
//	need += packStuffingLength
//	if s.Size() < need {
//		return ParseResult{State: ParseStateNotEnough, Need: uint32(need)}
//	}
//	h.packStartCode = binary.BigEndian.Uint32(data[:4])
//	if h.packStartCode != PackStartCode {
//		stream.Must(s.Skip(4))
//		return ParseResult{State: ParseStateError, ErrCode: PackHeaderStartCodeError2.ErrCode}
//	}
//	copy(h.bytes10[:], data[4:])
//	stream.Must(s.Skip(need))
//	return ParseResult{State: ParseStateComplete}
//}

func (h *PackHeader) PackStartCode() uint32 {
	return h.packStartCode
}

func (h *PackHeader) MPEGFlag() uint8 {
	return (h.bytes10[0] & 0b11000000) >> 6
}

func (h *PackHeader) SystemClockReferenceBase() uint64 {
	return (uint64(h.bytes10[0]&0b00111000) << 27) |
		(uint64(h.bytes10[0]&0b00000011) << 28) |
		(uint64(h.bytes10[1]) << 20) |
		(uint64(h.bytes10[2]&0b11111000) << 12) |
		(uint64(h.bytes10[2]&0b00000011) << 13) |
		(uint64(h.bytes10[3]) << 5) |
		(uint64(h.bytes10[4]) >> 3)
}

func (h *PackHeader) SystemClockReferenceExtension() uint16 {
	return (uint16(h.bytes10[4]&0b00000011) << 7) | (uint16(h.bytes10[5]&0b11111110) >> 1)
}

func (h *PackHeader) ProgramMuxRate() uint32 {
	return (uint32(h.bytes10[6]) << 14) | (uint32(h.bytes10[7] << 6)) | (uint32(h.bytes10[8]) >> 2)
}

func (h *PackHeader) Reserved() uint8 {
	return h.bytes10[9] >> 3
}

func (h *PackHeader) PackStuffingLength() uint8 {
	return h.bytes10[9] & 0b00000111
}

func (h *PackHeader) Size() uint {
	return 14 + uint(h.PackStuffingLength())
}

func (h *PackHeader) WriteTo(buf *bytes.Buffer) {
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, h.packStartCode)
	buf.Write(tmp)
	buf.Write(h.bytes10[:])
	buf.Write(PackHeaderStuffing[:h.PackStuffingLength()])
}
