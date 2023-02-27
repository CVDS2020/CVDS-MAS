package ps

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const (
	StreamIdProgramStreamMap = 0b10111100
	StreamIdPrivateStream1   = 0b10111101
	StreamIdPaddingStream    = 0b10111110
	StreamIdPrivateStream2   = 0b10111111
	StreamIdECMStream        = 0b11110000
	StreamIdEMMStream        = 0b11110001
	StreamIdITUH2220         = 0b11110010
	StreamIdISO13818_1AnnexA
	StreamIdISO13818_6DSMCCStream
	StreamIdISO13522Stream             = 0b11110011
	StreamIdITUH2221TypeA              = 0b11110100
	StreamIdITUH2221TypeB              = 0b11110101
	StreamIdITUH2221TypeC              = 0b11110110
	StreamIdITUH2221TypeD              = 0b11110111
	StreamIdITUH2221TypeE              = 0b11111000
	StreamIdAncillaryStream            = 0b11111001
	StreamIdISO14496SLPacketizedStream = 0b11111010
	StreamIdISO14496FlexMuxStream      = 0b11111010
	StreamIdProgramStreamDirectory     = 0b11111111
)

var PESPacketLengthNotEnoughError = errors.New("pes packet length not enough")
var PESPacketLengthInvalidError = errors.New("pes packet length invalid")
var PESHeaderDataLengthInvalidError = errors.New("pes header data length invalid")

type PESGenericPrefix struct {
	packetStartCodePrefix [3]byte
	streamId              uint8
	pesPacketLength       uint16
}

func (p *PESGenericPrefix) Clear() {
	p.streamId = 0
}

func (p *PESGenericPrefix) Empty() bool {
	return p.streamId == 0
}

//func (p *PESGenericPrefix) Parse(s *stream.QueueStream) ParseResult {
//	need := uint(6)
//	data, err := s.Peek(need)
//	if err != nil {
//		return ParseResult{State: ParseStateNotEnough, Need: uint32(need)}
//	}
//	p.pesPacketLength = binary.BigEndian.Uint16(data[4:])
//	need += uint(p.pesPacketLength)
//	if s.Size() < need {
//		return ParseResult{State: ParseStateNotEnough, Need: uint32(need)}
//	}
//
//	copy(p.packetStartCodePrefix[:], data)
//	if p.packetStartCodePrefix != [3]byte{0, 0, 1} {
//		stream.Must(s.Skip(1))
//		return ParseResult{State: ParseStateError, ErrCode: PacketStartCodePrefixError.ErrCode}
//	}
//	p.streamId = data[3]
//
//	stream.Must(s.Skip(6))
//	return ParseResult{State: ParseStateComplete, Need: uint32(need)}
//}

func (p *PESGenericPrefix) PacketStartCodePrefix() [3]byte {
	return p.packetStartCodePrefix
}

func (p *PESGenericPrefix) StreamId() uint8 {
	return p.streamId
}

func (p *PESGenericPrefix) PESPacketLength() uint16 {
	return p.pesPacketLength
}

func (p *PESGenericPrefix) Size() uint {
	return 6 + uint(p.pesPacketLength)
}

func (p *PESGenericPrefix) Write(sb *bytes.Buffer) {
	buf := [6]byte{}
	copy(buf[:], p.packetStartCodePrefix[:])
	buf[3] = p.streamId
	binary.BigEndian.PutUint16(buf[4:], p.pesPacketLength)
	sb.Write(buf[:])
}

const PESDisplay = "PES"

var PESStuffing = [16]byte{
	0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff,
}

type PES struct {
	PESGenericPrefix
	pesHeader struct {
		bytes2              [2]byte
		pesHeaderDataLength uint8
		pts                 [5]byte
		dts                 [5]byte
		escr                [6]byte
		esRate              [3]byte
		dsmTrickMode        uint8
		additionalCopyInfo  uint8
		crc                 uint16
		pesExtension        struct {
			base                         uint8
			privateData                  [16]byte
			packFieldLength              uint8
			packHeader                   []byte
			programPacketSequenceCounter uint8
			mpeg1mpeg2Identifier         uint8
			pSTDBuffer                   [2]byte
			pesExtensionFieldLength      uint8
			pesExtensionFieldReversed    []byte
		}
	}
	packDataLength uint16
	pesPacketData  [][]byte
}

//func (p *PES) Parse(s *stream.QueueStream) ParseResult {
//	result := p.PESGenericPrefix.Parse(s)
//	switch result.State {
//	case ParseStateNotEnough, ParseStateError:
//		return result
//	}
//
//	var plNeed uint16
//	var data []byte
//
//	switch p.streamId {
//	case StreamIdProgramStreamMap,
//		StreamIdPaddingStream,
//		StreamIdPrivateStream2,
//		StreamIdECMStream,
//		StreamIdEMMStream,
//		StreamIdProgramStreamDirectory,
//		StreamIdISO13818_6DSMCCStream,
//		StreamIdITUH2221TypeE:
//	default:
//		if plNeed += 3; p.pesPacketLength < plNeed {
//			return ParseResult{State: ParseStateError, ErrCode: PESPacketLengthError.ErrCode}
//		}
//		data = stream.MustBytes(s.Read(3))
//
//		header := &p.pesHeader
//		header.pesHeaderDataLength = data[2]
//		if plNeed += uint16(header.pesHeaderDataLength); p.pesPacketLength < plNeed {
//			return ParseResult{State: ParseStateError, ErrCode: PESPacketLengthError.ErrCode}
//		}
//		header.bytes2[0] = data[0]
//		header.bytes2[1] = data[1]
//		data = stream.MustBytes(s.Read(uint(header.pesHeaderDataLength)))
//
//		var hdlNeed uint8
//		if p.PTS_DTS_Flags() == 0b10 {
//			if hdlNeed += 5; header.pesHeaderDataLength < hdlNeed {
//				return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//			}
//			copy(header.pts[:], data)
//			data = data[5:]
//		} else if p.PTS_DTS_Flags() == 0b11 {
//			if hdlNeed += 10; header.pesHeaderDataLength < hdlNeed {
//				return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//			}
//			copy(header.pts[:], data)
//			copy(header.dts[:], data[5:])
//			data = data[10:]
//		}
//
//		if p.ESCRFlag() {
//			if hdlNeed += 6; header.pesHeaderDataLength < hdlNeed {
//				return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//			}
//			copy(header.escr[:], data)
//			data = data[6:]
//		}
//
//		if p.ESRateFlag() {
//			if hdlNeed += 3; header.pesHeaderDataLength < hdlNeed {
//				return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//			}
//			copy(header.esRate[:], data)
//			data = data[:3]
//		}
//
//		if p.DSMTrickModeFlag() {
//			if hdlNeed += 1; header.pesHeaderDataLength < hdlNeed {
//				return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//			}
//			header.dsmTrickMode = data[0]
//			data = data[1:]
//		}
//
//		if p.AdditionalCopyInfoFlag() {
//			if hdlNeed += 1; header.pesHeaderDataLength < hdlNeed {
//				return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//			}
//			header.additionalCopyInfo = data[0]
//			data = data[1:]
//		}
//
//		if p.PES_CRC_Flag() {
//			if hdlNeed += 2; header.pesHeaderDataLength < hdlNeed {
//				return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//			}
//			header.crc = binary.BigEndian.Uint16(data)
//			data = data[2:]
//		}
//
//		if p.PESExtensionFlag() {
//			if hdlNeed += 1; header.pesHeaderDataLength < hdlNeed {
//				return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//			}
//			extension := &header.pesExtension
//			extension.base = data[0]
//			data = data[1:]
//
//			if p.PESPrivateDataFlag() {
//				if hdlNeed += 16; header.pesHeaderDataLength < hdlNeed {
//					return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//				}
//				copy(extension.privateData[:], data)
//				data = data[16:]
//			}
//
//			if p.PackHeaderFieldFlag() {
//				if hdlNeed += 1; header.pesHeaderDataLength < hdlNeed {
//					return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//				}
//				extension.packFieldLength = data[0]
//				data = data[1:]
//
//				if hdlNeed += extension.packFieldLength; header.pesHeaderDataLength < hdlNeed {
//					return ParseResult{State: ParseStateError}
//				}
//				extension.packHeader = data[:extension.packFieldLength]
//				data = data[extension.packFieldLength:]
//			}
//
//			if p.ProgramPacketSequenceCounterFlag() {
//				if hdlNeed += 2; header.pesHeaderDataLength < hdlNeed {
//					return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//				}
//				extension.programPacketSequenceCounter = data[0]
//				extension.mpeg1mpeg2Identifier = data[1]
//				data = data[2:]
//			}
//
//			if p.P_STD_BufferFlag() {
//				if hdlNeed += 2; header.pesHeaderDataLength < hdlNeed {
//					return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//				}
//				extension.pSTDBuffer[0] = data[0]
//				extension.pSTDBuffer[1] = data[1]
//				data = data[2:]
//			}
//
//			if p.PESExtensionFlag2() {
//				if hdlNeed += 1; header.pesHeaderDataLength < hdlNeed {
//					return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//				}
//				extension.pesExtensionFieldLength = data[0]
//				data = data[1:]
//
//				if hdlNeed += extension.pesExtensionFieldLength; header.pesHeaderDataLength < hdlNeed {
//					return ParseResult{State: ParseStateError, ErrCode: PESHeaderDataLengthError.ErrCode}
//				}
//				extension.pesExtensionFieldReversed = data[:extension.pesExtensionFieldLength]
//				data = data[extension.pesExtensionFieldLength:]
//			}
//		}
//	}
//
//	p.packDataLength = p.pesPacketLength - plNeed
//	p.pesPacketData = stream.MustChunks(s.ReadChunks(uint(p.packDataLength), p.pesPacketData[:0]))
//
//	return ParseResult{State: ParseStateComplete}
//}

func (p *PES) PESScramblingControl() uint8 {
	return (p.pesHeader.bytes2[0] & 0b00110000) >> 4
}

func (p *PES) PESPriority() uint8 {
	return (p.pesHeader.bytes2[0] & 0b00001000) >> 3
}

func (p *PES) DataAlignmentIndicator() uint8 {
	return (p.pesHeader.bytes2[0] & 0b00000100) >> 2
}

func (p *PES) Copyright() uint8 {
	return (p.pesHeader.bytes2[0] & 0b00000010) >> 1
}

func (p *PES) OriginalOrCopy() uint8 {
	return p.pesHeader.bytes2[0] & 0b00000001
}

func (p *PES) PTS_DTS_Flags() uint8 {
	return (p.pesHeader.bytes2[1] & 0b11000000) >> 6
}

func (p *PES) ESCRFlag() bool {
	return p.pesHeader.bytes2[1]&0b00100000 != 0
}

func (p *PES) ESRateFlag() bool {
	return p.pesHeader.bytes2[1]&0b00010000 != 0
}

func (p *PES) DSMTrickModeFlag() bool {
	return p.pesHeader.bytes2[1]&0b00001000 != 0
}

func (p *PES) AdditionalCopyInfoFlag() bool {
	return p.pesHeader.bytes2[1]&0b00000100 != 0
}

func (p *PES) PES_CRC_Flag() bool {
	return p.pesHeader.bytes2[1]&0b00000010 != 0
}

func (p *PES) PESExtensionFlag() bool {
	return p.pesHeader.bytes2[1]&0b00000001 != 0
}

func (p *PES) PESHeaderDataLength() uint8 {
	return p.pesHeader.pesHeaderDataLength
}

func (p *PES) PTS() uint64 {
	return (uint64(p.pesHeader.pts[0]&0b00001110) << 29) |
		(uint64(p.pesHeader.pts[1]) << 22) |
		(uint64(p.pesHeader.pts[2]&0b11111110) << 14) |
		(uint64(p.pesHeader.pts[3]) << 7) |
		(uint64(p.pesHeader.pts[4]&0b11111110) >> 1)
}

func (p *PES) DTS() uint64 {
	return (uint64(p.pesHeader.dts[0]&0b00001110) << 29) |
		(uint64(p.pesHeader.dts[1]) << 22) |
		(uint64(p.pesHeader.dts[2]&0b11111110) << 14) |
		(uint64(p.pesHeader.dts[3]) << 7) |
		(uint64(p.pesHeader.dts[4]&0b11111110) >> 1)
}

func (p *PES) ESCRBase() uint64 {
	return (uint64(p.pesHeader.escr[0]&0b00111000) << 27) |
		(uint64(p.pesHeader.escr[0]&0b00000011) << 28) |
		(uint64(p.pesHeader.escr[1]) << 20) |
		(uint64(p.pesHeader.escr[2]&0b11111000) << 12) |
		(uint64(p.pesHeader.escr[2]&0b00000011) << 13) |
		(uint64(p.pesHeader.escr[3]) << 5) |
		(uint64(p.pesHeader.escr[4]&0b11111000) >> 3)
}

func (p *PES) ESCRExtension() uint16 {
	return (uint16(p.pesHeader.escr[4]&0b00000011) << 7) | (uint16(p.pesHeader.escr[5]&0b11111110) >> 1)
}

func (p *PES) ESRate() uint32 {
	return (uint32(p.pesHeader.esRate[0]&0b01111111) << 15) |
		(uint32(p.pesHeader.esRate[1]) << 7) |
		(uint32(p.pesHeader.esRate[2]&0b11111110) >> 1)
}

func (p *PES) TrickModeControl() uint8 {
	return (p.pesHeader.dsmTrickMode & 0b11100000) >> 5
}

func (p *PES) FieldId() uint8 {
	return (p.pesHeader.dsmTrickMode & 0b00011000) >> 3
}

func (p *PES) IntraSliceRefresh() uint8 {
	return (p.pesHeader.dsmTrickMode & 0b00000100) >> 2
}

func (p *PES) FrequencyTruncation() uint8 {
	return p.pesHeader.dsmTrickMode & 0b00000011
}

func (p *PES) RepCntrl() uint8 {
	return p.pesHeader.dsmTrickMode & 0b00011111
}

func (p *PES) AdditionalCopyInfo() uint8 {
	return p.pesHeader.additionalCopyInfo & 0b01111111
}

func (p *PES) PreviousPESPacketCRC() uint16 {
	return p.pesHeader.crc
}

func (p *PES) PESPrivateDataFlag() bool {
	return p.pesHeader.pesExtension.base&0b10000000 != 0
}

func (p *PES) PackHeaderFieldFlag() bool {
	return p.pesHeader.pesExtension.base&0b01000000 != 0
}

func (p *PES) ProgramPacketSequenceCounterFlag() bool {
	return p.pesHeader.pesExtension.base&0b00100000 != 0
}

func (p *PES) P_STD_BufferFlag() bool {
	return p.pesHeader.pesExtension.base&0b00010000 != 0
}

func (p *PES) PESExtensionFlag2() bool {
	return p.pesHeader.pesExtension.base&0b00000001 != 0
}

func (p *PES) PESPrivateData() [16]byte {
	return p.pesHeader.pesExtension.privateData
}

func (p *PES) PackFieldLength() uint8 {
	return p.pesHeader.pesExtension.packFieldLength
}

func (p *PES) PackHeader() []byte {
	return p.pesHeader.pesExtension.packHeader
}

func (p *PES) ProgramPacketSequenceCounter() uint8 {
	return p.pesHeader.pesExtension.programPacketSequenceCounter & 0b01111111
}

func (p *PES) MPEG1_MPEG2_Identifier() uint8 {
	return (p.pesHeader.pesExtension.mpeg1mpeg2Identifier & 0b01000000) >> 6
}

func (p *PES) P_STD_BufferScale() uint8 {
	return (p.pesHeader.pesExtension.pSTDBuffer[0] & 0b00100000) >> 5
}

func (p *PES) P_STD_BufferSize() uint16 {
	return (uint16(p.pesHeader.pesExtension.pSTDBuffer[0])&0b00011111)<<8 | uint16(p.pesHeader.pesExtension.pSTDBuffer[1])
}

func (p *PES) PESExtensionFieldLength() uint8 {
	return p.pesHeader.pesExtension.pesExtensionFieldLength
}

func (p *PES) PesExtensionFieldReversed() []byte {
	return p.pesHeader.pesExtension.pesExtensionFieldReversed
}

func (p *PES) WriteTo(buf *bytes.Buffer) {
	p.PESGenericPrefix.Write(buf)
	switch p.streamId {
	case StreamIdProgramStreamMap,
		StreamIdPaddingStream,
		StreamIdPrivateStream2,
		StreamIdECMStream,
		StreamIdEMMStream,
		StreamIdProgramStreamDirectory,
		StreamIdISO13818_6DSMCCStream,
		StreamIdITUH2221TypeE:
	default:
		header := &p.pesHeader
		buf.Write([]byte{header.bytes2[0], header.bytes2[1], header.pesHeaderDataLength})
		l := buf.Len()
		if p.PTS_DTS_Flags() == 0b10 {
			buf.Write(header.pts[:])
		} else if p.PTS_DTS_Flags() == 0b11 {
			buf.Write(header.pts[:])
			buf.Write(header.dts[:])
		}
		if p.ESCRFlag() {
			buf.Write(header.escr[:])
		}
		if p.ESRateFlag() {
			buf.Write(header.esRate[:])
		}
		if p.DSMTrickModeFlag() {
			buf.WriteByte(header.dsmTrickMode)
		}
		if p.AdditionalCopyInfoFlag() {
			buf.WriteByte(header.additionalCopyInfo)
		}
		if p.PES_CRC_Flag() {
			tmp := make([]byte, 2)
			binary.BigEndian.PutUint16(tmp, header.crc)
			buf.Write(tmp)
		}
		if p.PESExtensionFlag() {
			extension := &header.pesExtension
			buf.WriteByte(extension.base)
			if p.PESPrivateDataFlag() {
				buf.Write(extension.privateData[:])
			}
			if p.PackHeaderFieldFlag() {
				buf.WriteByte(extension.packFieldLength)
				buf.Write(extension.packHeader[:extension.packFieldLength])
			}
			if p.ProgramPacketSequenceCounterFlag() {
				buf.WriteByte(extension.programPacketSequenceCounter)
				buf.WriteByte(extension.mpeg1mpeg2Identifier)
			}
			if p.P_STD_BufferFlag() {
				buf.Write(extension.pSTDBuffer[:])
			}
			if p.PESExtensionFlag2() {
				buf.WriteByte(extension.pesExtensionFieldLength)
				buf.Write(extension.pesExtensionFieldReversed[:extension.pesExtensionFieldLength])
			}
		}
		stuffingLength := int(header.pesHeaderDataLength) - (buf.Len() - l)
		if stuffingLength < 0 {
			panic(PESPacketLengthInvalidError.Error())
		}
		for stuffingLength > 0 {
			if stuffingLength <= len(PESStuffing) {
				buf.Write(PESStuffing[:stuffingLength])
				break
			}
			buf.Write(PESStuffing[:])
			stuffingLength -= len(PESStuffing)
		}
	}
	for _, chunk := range p.pesPacketData {
		buf.Write(chunk)
	}
}
