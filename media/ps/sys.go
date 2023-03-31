package ps

import "io"

type SysStreamInfo struct {
	streamId uint8
	bytes2   [2]byte
}

func (i *SysStreamInfo) StreamId() uint8 {
	return i.streamId
}

func (i *SysStreamInfo) PSTDBufferBoundScale() uint8 {
	return (i.bytes2[0] & 0b00100000) >> 5
}

func (i *SysStreamInfo) PSTDBufferSizeBound() uint16 {
	return (uint16(i.bytes2[0]&0b00011111) << 8) | uint16(i.bytes2[1])
}

func (i *SysStreamInfo) WriteTo(w io.Writer) (n int64, err error) {
	buf := [3]byte{i.streamId, i.bytes2[0], i.bytes2[1]}
	err = Write(w, buf[:], &n)
	return
}

type SYS struct {
	startCode    [4]byte
	headerLength uint16
	bytes6       [6]byte
	streamInfos  []SysStreamInfo
}

func (s *SYS) StartCode() [4]byte {
	return s.startCode
}

func (s *SYS) HeaderLength() uint16 {
	return s.headerLength
}

func (s *SYS) RateBound() uint32 {
	return (uint32(s.bytes6[0]&0b01111111) << 15) | (uint32(s.bytes6[1]) << 7) | (uint32(s.bytes6[2] >> 1))
}

func (s *SYS) AudioBound() uint8 {
	return s.bytes6[3] >> 2
}

func (s *SYS) FixedFlag() bool {
	return s.bytes6[3]&0b00000010 != 0
}

func (s *SYS) CSPSFlag() bool {
	return s.bytes6[3]&0b00000001 != 0
}

func (s *SYS) SystemAudioLockFlag() bool {
	return s.bytes6[4]&0b10000000 != 0
}

func (s *SYS) SystemVideoLockFlag() bool {
	return s.bytes6[4]&0b01000000 != 0
}

func (s *SYS) VideoBound() uint8 {
	return s.bytes6[4] & 0b00011111
}

func (s *SYS) PacketRateRestrictionFlag() bool {
	return s.bytes6[5]&0b10000000 != 0
}

func (s *SYS) ReversedBits() uint8 {
	return s.bytes6[5] >> 1
}

func (s *SYS) StreamInfos() []SysStreamInfo {
	return s.streamInfos
}

func (s *SYS) Size() int {
	if s.startCode[3] == 0 {
		return 0
	}
	return 6 + int(s.headerLength)
}

func (s *SYS) WriteTo(w io.Writer) (n int64, err error) {
	if s.startCode[3] == 0 {
		return 0, nil
	}
	buf := [16]byte{}
	writer := Writer{Buf: buf[:]}
	if err = WriteAndReset(writer.WriteBytesAnd(s.startCode[:]).
		WriteUint16(s.headerLength).
		WriteBytesAnd(s.bytes6[:]), w, &n); err != nil {
		return
	}
	for i := range s.streamInfos {
		if err = WriteTo(&s.streamInfos[i], w, &n); err != nil {
			return
		}
	}
	return
}

func (s *SYS) Clear() {
	s.startCode = [4]byte{}
	s.headerLength = 0
	s.streamInfos = s.streamInfos[:0]
}
