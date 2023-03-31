package ps

import (
	"errors"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"gitee.com/sy_183/cvds-mas/media"
	"io"
)

var PESPacketLengthNotEnoughError = errors.New("pes packet length not enough")
var PESPacketLengthInvalidError = errors.New("pes packet length invalid")
var PESHeaderDataLengthInvalidError = errors.New("pes header data length invalid")

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

	StreamTypeVideoMPEG1 = 0x01
	StreamTypeVideoMPEG2 = 0x02
	StreamTypeAudioMPEG1 = 0x03
	StreamTypeAudioMPEG2 = 0x04
	StreamTypeAudioAAC   = 0x0f
	StreamTypeVideoMPEG4 = 0x10
	StreamTypeVideoH264  = 0x1B
	StreamTypeVideoH265  = 0x24
	StreamTypeVideoSVAC  = 0x80
	StreamTypeAudioAC3   = 0x81
	StreamTypeAudioDTS   = 0x8a
	StreamTypeAudioLPCM  = 0x8b
	StreamTypeAudioG711A = 0x90
	StreamTypeAudioG711U = 0x91
	StreamTypeAudioG7221 = 0x92
	StreamTypeAudioG7231 = 0x93
	StreamTypeAudioG729  = 0x99
	StreamTypeAudioSVAC  = 0x9B
)

var streamTypeMediaTypeMap = map[uint8]*media.MediaType{
	StreamTypeAudioAAC:   &media.MediaTypeAAC,
	StreamTypeVideoMPEG4: &media.MediaTypeMPEG4,
	StreamTypeVideoH264:  &media.MediaTypeH264,
	StreamTypeVideoH265:  &media.MediaTypeH265,
	StreamTypeVideoSVAC:  &media.MediaTypeSVAC,
	StreamTypeAudioG711A: &media.MediaTypeG711A,
	StreamTypeAudioG7221: &media.MediaTypeG7221,
	StreamTypeAudioG7231: &media.MediaTypeG7231,
	StreamTypeAudioG729:  &media.MediaTypeG729,
	StreamTypeAudioSVAC:  &media.MediaTypeSVACA,
}

func StreamTypeToMediaType(streamType uint8) *media.MediaType {
	return streamTypeMediaTypeMap[streamType]
}

var PESStuffing = [16]byte{
	0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff,
}

type PESHeader struct {
	startCode          [4]byte
	packetLength       uint16
	bytes2             [2]byte
	headerDataLength   uint8
	pts                [5]byte
	dts                [5]byte
	escr               [6]byte
	esRate             [3]byte
	dsmTrickMode       uint8
	additionalCopyInfo uint8
	crc                uint16
	extension          struct {
		base                         uint8
		privateData                  [16]byte
		packFieldLength              uint8
		packHeader                   []byte
		programPacketSequenceCounter uint8
		mpeg1mpeg2Identifier         uint8
		pSTDBuffer                   [2]byte
		extensionFieldLength         uint8
		extensionFieldReversed       []byte
	}
}

func (p *PESHeader) StartCode() [4]byte {
	return p.startCode
}

func (p *PESHeader) StreamId() uint8 {
	return p.startCode[3]
}

func (p *PESHeader) PacketLength() uint16 {
	return p.packetLength
}

func (p *PESHeader) ScramblingControl() uint8 {
	return (p.bytes2[0] & 0b00110000) >> 4
}

func (p *PESHeader) Priority() uint8 {
	return (p.bytes2[0] & 0b00001000) >> 3
}

func (p *PESHeader) DataAlignmentIndicator() uint8 {
	return (p.bytes2[0] & 0b00000100) >> 2
}

func (p *PESHeader) Copyright() uint8 {
	return (p.bytes2[0] & 0b00000010) >> 1
}

func (p *PESHeader) OriginalOrCopy() uint8 {
	return p.bytes2[0] & 0b00000001
}

func (p *PESHeader) PTS_DTS_Flags() uint8 {
	return (p.bytes2[1] & 0b11000000) >> 6
}

func (p *PESHeader) ESCRFlag() bool {
	return p.bytes2[1]&0b00100000 != 0
}

func (p *PESHeader) ESRateFlag() bool {
	return p.bytes2[1]&0b00010000 != 0
}

func (p *PESHeader) DSMTrickModeFlag() bool {
	return p.bytes2[1]&0b00001000 != 0
}

func (p *PESHeader) AdditionalCopyInfoFlag() bool {
	return p.bytes2[1]&0b00000100 != 0
}

func (p *PESHeader) CRCFlag() bool {
	return p.bytes2[1]&0b00000010 != 0
}

func (p *PESHeader) ExtensionFlag() bool {
	return p.bytes2[1]&0b00000001 != 0
}

func (p *PESHeader) HeaderDataLength() uint8 {
	return p.headerDataLength
}

func (p *PESHeader) PTS() uint64 {
	return (uint64(p.pts[0]&0b00001110) << 29) |
		(uint64(p.pts[1]) << 22) |
		(uint64(p.pts[2]&0b11111110) << 14) |
		(uint64(p.pts[3]) << 7) |
		(uint64(p.pts[4]&0b11111110) >> 1)
}

func (p *PESHeader) DTS() uint64 {
	return (uint64(p.dts[0]&0b00001110) << 29) |
		(uint64(p.dts[1]) << 22) |
		(uint64(p.dts[2]&0b11111110) << 14) |
		(uint64(p.dts[3]) << 7) |
		(uint64(p.dts[4]&0b11111110) >> 1)
}

func (p *PESHeader) ESCRBase() uint64 {
	return (uint64(p.escr[0]&0b00111000) << 27) |
		(uint64(p.escr[0]&0b00000011) << 28) |
		(uint64(p.escr[1]) << 20) |
		(uint64(p.escr[2]&0b11111000) << 12) |
		(uint64(p.escr[2]&0b00000011) << 13) |
		(uint64(p.escr[3]) << 5) |
		(uint64(p.escr[4]&0b11111000) >> 3)
}

func (p *PESHeader) ESCRExtension() uint16 {
	return (uint16(p.escr[4]&0b00000011) << 7) | (uint16(p.escr[5]&0b11111110) >> 1)
}

func (p *PESHeader) ESRate() uint32 {
	return (uint32(p.esRate[0]&0b01111111) << 15) |
		(uint32(p.esRate[1]) << 7) |
		(uint32(p.esRate[2]&0b11111110) >> 1)
}

func (p *PESHeader) TrickModeControl() uint8 {
	return (p.dsmTrickMode & 0b11100000) >> 5
}

func (p *PESHeader) FieldId() uint8 {
	return (p.dsmTrickMode & 0b00011000) >> 3
}

func (p *PESHeader) IntraSliceRefresh() uint8 {
	return (p.dsmTrickMode & 0b00000100) >> 2
}

func (p *PESHeader) FrequencyTruncation() uint8 {
	return p.dsmTrickMode & 0b00000011
}

func (p *PESHeader) RepCntrl() uint8 {
	return p.dsmTrickMode & 0b00011111
}

func (p *PESHeader) AdditionalCopyInfo() uint8 {
	return p.additionalCopyInfo & 0b01111111
}

func (p *PESHeader) PreviousPESPacketCRC() uint16 {
	return p.crc
}

func (p *PESHeader) PrivateDataFlag() bool {
	return p.extension.base&0b10000000 != 0
}

func (p *PESHeader) PackHeaderFieldFlag() bool {
	return p.extension.base&0b01000000 != 0
}

func (p *PESHeader) ProgramPacketSequenceCounterFlag() bool {
	return p.extension.base&0b00100000 != 0
}

func (p *PESHeader) P_STD_BufferFlag() bool {
	return p.extension.base&0b00010000 != 0
}

func (p *PESHeader) ExtensionFlag2() bool {
	return p.extension.base&0b00000001 != 0
}

func (p *PESHeader) PrivateData() [16]byte {
	return p.extension.privateData
}

func (p *PESHeader) PackFieldLength() uint8 {
	return p.extension.packFieldLength
}

func (p *PESHeader) PackHeader() []byte {
	return p.extension.packHeader
}

func (p *PESHeader) ProgramPacketSequenceCounter() uint8 {
	return p.extension.programPacketSequenceCounter & 0b01111111
}

func (p *PESHeader) MPEG1_MPEG2_Identifier() uint8 {
	return (p.extension.mpeg1mpeg2Identifier & 0b01000000) >> 6
}

func (p *PESHeader) P_STD_BufferScale() uint8 {
	return (p.extension.pSTDBuffer[0] & 0b00100000) >> 5
}

func (p *PESHeader) P_STD_BufferSize() uint16 {
	return (uint16(p.extension.pSTDBuffer[0])&0b00011111)<<8 | uint16(p.extension.pSTDBuffer[1])
}

func (p *PESHeader) ExtensionFieldLength() uint8 {
	return p.extension.extensionFieldLength
}

func (p *PESHeader) ExtensionFieldReversed() []byte {
	return p.extension.extensionFieldReversed
}

func (p *PESHeader) Size() int {
	if p.startCode[3] == 0 {
		return 0
	}
	return int(9 + p.headerDataLength)
}

func (p *PESHeader) WriteTo(w io.Writer) (n int64, err error) {
	if p.startCode[3] == 0 {
		return 0, nil
	}

	buf := [16]byte{}
	writer := ioUtils.Writer{Buf: buf[:]}

	defer func() { err = ioUtils.HandleRecovery(recover()) }()
	ioUtils.WriteAndResetPanic(writer.WriteBytesAnd(p.startCode[:]).WriteUint16(p.packetLength), w, &n)

	switch p.StreamId() {
	case StreamIdProgramStreamMap,
		StreamIdPaddingStream,
		StreamIdPrivateStream2,
		StreamIdECMStream,
		StreamIdEMMStream,
		StreamIdProgramStreamDirectory,
		StreamIdISO13818_6DSMCCStream,
		StreamIdITUH2221TypeE:
	default:
		ioUtils.WriteAndResetPanic(writer.WriteBytesAnd(p.bytes2[:]).WriteByte(p.headerDataLength), w, &n)
		cn := n

		if p.PTS_DTS_Flags() == 0b10 {
			writer.WriteBytes(p.pts[:])
		} else if p.PTS_DTS_Flags() == 0b11 {
			writer.WriteBytesAnd(p.pts[:]).WriteBytes(p.dts[:])
		}
		ioUtils.WriteAndResetPanic(&writer, w, &n)

		if p.ESCRFlag() {
			writer.WriteBytes(p.escr[:])
		}
		if p.ESRateFlag() {
			writer.WriteBytes(p.esRate[:])
		}
		if p.DSMTrickModeFlag() {
			writer.WriteByte(p.dsmTrickMode)
		}
		if p.AdditionalCopyInfoFlag() {
			writer.WriteByte(p.additionalCopyInfo)
		}
		if p.CRCFlag() {
			writer.WriteUint16(p.crc)
		}
		ioUtils.WriteAndResetPanic(&writer, w, &n)

		if p.ExtensionFlag() {
			extension := &p.extension
			writer.WriteByte(extension.base)
			if p.PrivateDataFlag() {
				ioUtils.WriteAndResetPanic(&writer, w, &n)
				ioUtils.WritePanic(w, extension.privateData[:], &n)
			}
			if p.PackHeaderFieldFlag() {
				ioUtils.WriteAndResetPanic(writer.WriteByte(extension.packFieldLength), w, &n)
				ioUtils.WritePanic(w, extension.packHeader[:extension.packFieldLength], &n)
			}
			if p.ProgramPacketSequenceCounterFlag() {
				writer.WriteByte(extension.programPacketSequenceCounter).WriteByte(extension.mpeg1mpeg2Identifier)
			}
			if p.P_STD_BufferFlag() {
				writer.WriteBytes(extension.pSTDBuffer[:])
			}
			if p.ExtensionFlag2() {
				ioUtils.WriteAndResetPanic(writer.WriteByte(extension.extensionFieldLength), w, &n)
				ioUtils.WritePanic(w, extension.extensionFieldReversed[:extension.extensionFieldLength], &n)
			}
		}

		stuffingLength := int(p.headerDataLength) - int(n-cn)
		if stuffingLength < 0 {
			panic(PESPacketLengthInvalidError)
		}
		for stuffingLength > 0 {
			if stuffingLength <= len(PESStuffing) {
				ioUtils.WritePanic(w, PESStuffing[:stuffingLength], &n)
				break
			}
			ioUtils.WritePanic(w, PESStuffing[:], &n)
			stuffingLength -= len(PESStuffing)
		}
	}
	return
}

func (p *PESHeader) Clear() {
	p.startCode = [4]byte{}
	p.packetLength = 0
	p.bytes2 = [2]byte{}
	p.headerDataLength = 0
	p.extension.base = 0
	p.extension.packFieldLength = 0
	p.extension.extensionFieldLength = 0
}

type PESPacketData struct {
	chunks [][]byte
	size   int
}

func (b *PESPacketData) Append(chunk []byte) {
	b.chunks = append(b.chunks, chunk)
	b.size += len(chunk)
}

func (b *PESPacketData) Range(f func(chunk []byte) bool) bool {
	for _, chunk := range b.chunks {
		if !f(chunk) {
			return false
		}
	}
	return true
}

func (b *PESPacketData) Size() int {
	return b.size
}

func (b *PESPacketData) Bytes() []byte {
	w := ioUtils.Writer{Buf: make([]byte, b.Size())}
	n, _ := b.WriteTo(&w)
	return w.Buf[:n]
}

func (b *PESPacketData) Read(buf []byte) (n int, err error) {
	w := ioUtils.Writer{Buf: buf}
	_n, _ := b.WriteTo(&w)
	return int(_n), io.EOF
}

func (b *PESPacketData) WriteTo(w io.Writer) (n int64, err error) {
	for _, content := range b.chunks {
		if err = ioUtils.Write(w, content, &n); err != nil {
			return
		}
	}
	return
}

func (b *PESPacketData) Clear() {
	b.chunks = b.chunks[:0]
}

type PES struct {
	PESHeader
	packetData PESPacketData
}

func (p *PES) Header() *PESHeader {
	return &p.PESHeader
}

func (p *PES) PackageData() *PESPacketData {
	return &p.packetData
}

func (p *PES) Size() int {
	if p.startCode[3] == 0 {
		return 0
	}
	return int(6 + p.packetLength)
}

func (p *PES) WriteTo(w io.Writer) (n int64, err error) {
	if err = ioUtils.WriteTo(&p.PESHeader, w, &n); err != nil {
		return
	}
	p.packetData.Range(func(chunk []byte) bool {
		if err = ioUtils.Write(w, chunk, &n); err != nil {
			return false
		}
		return true
	})
	return
}

func (p *PES) Clear() {
	p.PESHeader.Clear()
	p.packetData = PESPacketData{}
}
