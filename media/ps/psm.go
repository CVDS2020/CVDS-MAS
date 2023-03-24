package ps

import (
	"encoding/binary"
	"io"
)

type ElementaryStreamInfo struct {
	typ         uint8
	id          uint8
	infoLength  uint16
	descriptors []byte
}

func (i *ElementaryStreamInfo) Type() uint8 {
	return i.typ
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

func (i *ElementaryStreamInfo) Size() uint {
	return uint(4 + len(i.descriptors))
}

func (i *ElementaryStreamInfo) WriteTo(w io.Writer) (n int64, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()
	buf := [4]byte{i.typ, i.id}
	binary.BigEndian.PutUint16(buf[2:], i.infoLength)
	WritePanic(w, buf[:], &n)
	WritePanic(w, i.descriptors[:i.infoLength], &n)
	return
}

type PSM struct {
	startCode                 [4]byte
	packetLength              uint16
	bytes2                    [2]byte
	programStreamInfoLength   uint16
	descriptors               []byte
	elementaryStreamMapLength uint16
	elementaryStreamInfos     []ElementaryStreamInfo
	crc32                     uint32
}

func (p *PSM) StartCode() [4]byte {
	return p.startCode
}

func (p *PSM) StreamId() uint8 {
	return p.startCode[3]
}

func (p *PSM) PacketLength() uint16 {
	return p.packetLength
}

func (p *PSM) CurrentNextIndicator() uint8 {
	return (p.bytes2[0] & 0b10000000) >> 7
}

func (p *PSM) ProgramStreamMapVersion() uint8 {
	return p.bytes2[0] & 0b00011111
}

func (p *PSM) ProgramStreamInfoLength() uint16 {
	return p.programStreamInfoLength
}

func (p *PSM) Descriptors() []byte {
	return p.descriptors
}

func (p *PSM) ElementaryStreamInfos() []ElementaryStreamInfo {
	return p.elementaryStreamInfos
}

func (p *PSM) CRC32() uint32 {
	return p.crc32
}

func (p *PSM) Size() uint {
	if p.startCode[3] == 0 {
		return 0
	}
	return uint(6 + p.packetLength)
}

func (p *PSM) WriteTo(w io.Writer) (n int64, err error) {
	if p.startCode[3] == 0 {
		return 0, nil
	}
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()

	buf := [16]byte{}
	writer := Writer{Buf: buf[:]}

	WriteAndResetPanic(writer.WriteBytesAnd(p.startCode[:]).
		WriteUint16(p.packetLength).
		WriteBytesAnd(p.bytes2[:]).
		WriteUint16(p.programStreamInfoLength), w, &n)
	WritePanic(w, p.descriptors[:p.programStreamInfoLength], &n)
	WriteAndResetPanic(writer.WriteUint16(p.elementaryStreamMapLength), w, &n)
	for i := uint16(0); i < p.elementaryStreamMapLength; i++ {
		WriteToPanic(&p.elementaryStreamInfos[i], w, &n)
	}
	WriteAndResetPanic(writer.WriteUint32(p.crc32), w, &n)
	return
}

func (p *PSM) Clear() {
	p.startCode = [4]byte{}
	p.packetLength = 0
	p.programStreamInfoLength = 0
	p.elementaryStreamMapLength = 0
	p.elementaryStreamInfos = p.elementaryStreamInfos[:0]
}
