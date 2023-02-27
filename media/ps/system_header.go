package ps

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const SystemHeaderStartCode = 0x000001BB
const SystemHeaderDisplay = "ps system header"

type StreamInfo struct {
	streamId uint8
	bytes2   [2]byte
}

func (i *StreamInfo) StreamId() uint8 {
	return i.streamId
}

func (i *StreamInfo) PSTDBufferBoundScale() uint8 {
	return (i.bytes2[0] & 0b00100000) >> 5
}

func (i *StreamInfo) PSTDBufferSizeBound() uint16 {
	return (uint16(i.bytes2[0]&0b00011111) << 8) | uint16(i.bytes2[1])
}

func (i *StreamInfo) Write(sb *bytes.Buffer) {
	sb.WriteByte(i.streamId)
	sb.Write(i.bytes2[:])
}

type SystemHeader struct {
	systemHeaderStartCode uint32
	headerLength          uint16
	bytes6                [6]byte
	streamInfos           []StreamInfo
}

var SystemHeaderStartCodeError = fmt.Errorf("ps system header start code must be %08X", SystemHeaderStartCode)
var SystemHeaderLengthNotEnoughError = errors.New("ps system header length not enough")
var SystemHeaderLengthInvalidError = errors.New("ps system header field header_length invalid")

func (h *SystemHeader) initStreamInfos(length int) {
	if cap(h.streamInfos) >= length {
		h.streamInfos = h.streamInfos[:length]
	} else if cap(h.streamInfos)*2 < length {
		h.streamInfos = make([]StreamInfo, length)
	} else {
		h.streamInfos = make([]StreamInfo, length, 2*cap(h.streamInfos))
	}
}

func (h *SystemHeader) Clear() {
	h.systemHeaderStartCode = 0
}

//func (h *SystemHeader) Parse(s *stream.QueueStream) ParseResult {
//	// check length
//	need := uint(12)
//	data, err := s.Peek(need)
//	if err != nil {
//		return ParseResult{State: ParseStateNotEnough, Need: uint32(need)}
//	}
//
//	headerLength := binary.BigEndian.Uint16(data[4:])
//	if uint(headerLength)+6 < need {
//		stream.Must(s.Skip(4))
//		return ParseResult{State: ParseStateError, ErrCode: SystemHeaderLengthError.ErrCode}
//	}
//	need = uint(headerLength) + 6
//	siDataLength := need - 12
//	if s.Size() < need {
//		return ParseResult{State: ParseStateNotEnough, Need: uint32(need)}
//	} else if siDataLength%3 != 0 {
//		stream.Must(s.Skip(4))
//		return ParseResult{State: ParseStateError, ErrCode: SystemHeaderLengthError.ErrCode}
//	}
//
//	// populate field
//	h.systemHeaderStartCode = binary.BigEndian.Uint32(data)
//	if h.systemHeaderStartCode != SystemHeaderStartCode {
//		stream.Must(s.Skip(4))
//		return ParseResult{State: ParseStateError, ErrCode: SystemHeaderStartCodeError2.ErrCode}
//	}
//	h.headerLength = headerLength
//	copy(h.bytes6[:], data[6:])
//
//	stream.Must(s.Skip(12))
//	siData := stream.MustBytes(s.Read(siDataLength))
//	siLength := siDataLength / 3
//	h.initStreamInfos(int(siLength))
//	for i := uint(0); i < siLength; i++ {
//		h.streamInfos[i].streamId = siData[0]
//		h.streamInfos[i].bytes2[0] = siData[1]
//		h.streamInfos[i].bytes2[1] = siData[2]
//		siData = siData[3:]
//	}
//
//	return ParseResult{State: ParseStateComplete}
//}

func (h *SystemHeader) SystemHeaderStartCode() uint32 {
	return h.systemHeaderStartCode
}

func (h *SystemHeader) HeaderLength() uint16 {
	return h.headerLength
}

func (h *SystemHeader) RateBound() uint32 {
	return (uint32(h.bytes6[0]&0b01111111) << 15) | (uint32(h.bytes6[1]) << 7) | (uint32(h.bytes6[2] >> 1))
}

func (h *SystemHeader) AudioBound() uint8 {
	return h.bytes6[3] >> 2
}

func (h *SystemHeader) FixedFlag() bool {
	return h.bytes6[3]&0b00000010 != 0
}

func (h *SystemHeader) CSPSFlag() bool {
	return h.bytes6[3]&0b00000001 != 0
}

func (h *SystemHeader) SystemAudioLockFlag() bool {
	return h.bytes6[4]&0b10000000 != 0
}

func (h *SystemHeader) SystemVideoLockFlag() bool {
	return h.bytes6[4]&0b01000000 != 0
}

func (h *SystemHeader) VideoBound() uint8 {
	return h.bytes6[4] & 0b00011111
}

func (h *SystemHeader) PacketRateRestrictionFlag() bool {
	return h.bytes6[5]&0b10000000 != 0
}

func (h *SystemHeader) ReversedBits() uint8 {
	return h.bytes6[5] >> 1
}

func (h *SystemHeader) StreamInfos() []StreamInfo {
	return h.streamInfos
}

func (h *SystemHeader) Size() uint {
	return 6 + uint(h.headerLength)
}

func (h *SystemHeader) WriteTo(buf *bytes.Buffer) {
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, h.systemHeaderStartCode)
	buf.Write(tmp)
	binary.BigEndian.PutUint16(tmp, h.headerLength)
	buf.Write(tmp[:2])
	buf.Write(h.bytes6[:])
	for _, info := range h.streamInfos {
		info.Write(buf)
	}
}
