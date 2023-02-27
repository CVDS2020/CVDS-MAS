package ps

import (
	"bytes"
	"encoding/binary"
	"errors"
)

var ElementaryStreamMapLengthInvalidError = errors.New("elementary stream map length invalid")
var PSMStreamIdError = errors.New("PSM stream id error")

type ElementaryStreamInfo struct {
	_type       uint8
	id          uint8
	infoLength  uint16
	descriptors []byte
}

func (i *ElementaryStreamInfo) Type() uint8 {
	return i._type
}

func (i *ElementaryStreamInfo) Id() uint8 {
	return i.id
}

func (i *ElementaryStreamInfo) InfoLength() uint16 {
	return i.infoLength
}

func (i *ElementaryStreamInfo) Descriptors() []byte {
	return i.descriptors
}

func (i *ElementaryStreamInfo) Write(sb *bytes.Buffer) {
	sb.WriteByte(i._type)
	sb.WriteByte(i.id)
	tmp := make([]byte, 2)
	binary.BigEndian.PutUint16(tmp, i.infoLength)
	sb.Write(tmp)
	sb.Write(i.descriptors[:i.infoLength])
}

const PSMDisplay = "PSM"

type PSM struct {
	PESGenericPrefix
	bytes2                    [2]byte
	programStreamInfoLength   uint16
	descriptors               []byte
	elementaryStreamMapLength uint16
	elementaryStreamInfos     []ElementaryStreamInfo
	crc32                     uint32
}

//func (m *PSM) Parse(s *stream.QueueStream) ParseResult {
//	result := m.PESGenericPrefix.Parse(s)
//	switch result.State {
//	case ParseStateNotEnough, ParseStateError:
//		return result
//	}
//
//	if m.streamId != StreamIdProgramStreamMap {
//		return ParseResult{State: ParseStateError, ErrCode: PSMStreamIdError2.ErrCode}
//	}
//
//	need2 := uint16(10)
//	if m.pesPacketLength < need2 {
//		return ParseResult{State: ParseStateError, ErrCode: PESPacketLengthError.ErrCode}
//	}
//
//	data := stream.MustBytes(s.Read(4))
//	m.programStreamInfoLength = binary.BigEndian.Uint16(data[2:])
//	if need2 += m.programStreamInfoLength; m.pesPacketLength < need2 {
//		return ParseResult{State: ParseStateError, ErrCode: PESPacketLengthError.ErrCode}
//	}
//	m.bytes2[0], m.bytes2[1] = data[0], data[1]
//
//	data = stream.MustBytes(s.Read(uint(m.programStreamInfoLength) + 2))
//	m.descriptors = data[:m.programStreamInfoLength]
//	m.elementaryStreamMapLength = binary.BigEndian.Uint16(data[m.programStreamInfoLength:])
//	if need2 += m.elementaryStreamMapLength; m.pesPacketLength < need2 {
//		return ParseResult{State: ParseStateError, ErrCode: PESPacketLengthError.ErrCode}
//	}
//
//	data = stream.MustBytes(s.Read(uint(m.elementaryStreamMapLength) + 4))
//	m.elementaryStreamInfos = m.elementaryStreamInfos[:0]
//	var need3 uint16
//	for len(data) > 4 {
//		info := ElementaryStreamInfo{}
//		if need3 += 4; m.elementaryStreamMapLength < need3 {
//			return ParseResult{State: ParseStateError, ErrCode: ElementaryStreamMapLengthError.ErrCode}
//		}
//		info.infoLength = binary.BigEndian.Uint16(data[2:])
//		if need3 += info.infoLength; m.elementaryStreamMapLength < need3 {
//			return ParseResult{State: ParseStateError, ErrCode: ElementaryStreamMapLengthError.ErrCode}
//		}
//		info._type = data[0]
//		info.id = data[1]
//		info.descriptors = data[4 : 4+info.infoLength]
//		m.elementaryStreamInfos = append(m.elementaryStreamInfos, info)
//		data = data[4+info.infoLength:]
//	}
//
//	m.crc32 = binary.BigEndian.Uint32(data)
//
//	return ParseResult{State: ParseStateComplete}
//}

func (m *PSM) CurrentNextIndicator() uint8 {
	return (m.bytes2[0] & 0b10000000) >> 7
}

func (m *PSM) ProgramStreamMapVersion() uint8 {
	return m.bytes2[0] & 0b00011111
}

func (m *PSM) ProgramStreamInfoLength() uint16 {
	return m.programStreamInfoLength
}

func (m *PSM) Descriptors() []byte {
	return m.descriptors
}

func (m *PSM) ElementaryStreamInfos() []ElementaryStreamInfo {
	return m.elementaryStreamInfos
}

func (m *PSM) CRC32() uint32 {
	return m.crc32
}

func (m *PSM) WriteTo(buf *bytes.Buffer) {
	m.PESGenericPrefix.Write(buf)
	tmp := make([]byte, 4)
	buf.Write(m.bytes2[:])
	binary.BigEndian.PutUint16(tmp, m.programStreamInfoLength)
	buf.Write(tmp[:2])
	buf.Write(m.descriptors[:m.programStreamInfoLength])
	binary.BigEndian.PutUint16(tmp, m.elementaryStreamMapLength)
	buf.Write(tmp[:2])
	for _, info := range m.elementaryStreamInfos {
		info.Write(buf)
	}
	binary.BigEndian.PutUint32(tmp, m.crc32)
	buf.Write(tmp)
}
