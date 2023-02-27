package ps

import (
	"bytes"
	"encoding/binary"
)

const (
	stateParseStartCodePrefix = iota
	stateParseStartCodeSuffix
	stateParsePackHeader
	stateParseSystemHeader
	stateParseSystemHeaderStreamInfos
	stateParsePSM
	stateParsePSM_PESHeader
	stateParsePES
)

var startCodePrefixArray = [3]byte{0, 0, 1}
var startCodePrefix = startCodePrefixArray[:]

type readContext struct {
	len int
	buf []byte
}

func (c *readContext) reset(len int) *readContext {
	c.len = len
	c.buf = c.buf[:0]
	return c
}

func (c *readContext) read(buf []byte) (remain []byte, ok bool) {
	if c.len == 0 {
		return buf, true
	}
	if c.len <= len(buf) {
		if len(c.buf) == 0 {
			c.buf, remain = buf[:c.len], buf[c.len:]
			c.len = 0
		} else {
			c.buf, remain = append(c.buf, buf[:c.len]...), buf[c.len:]
			c.len = 0
		}
		return remain, true
	}
	c.buf = append(c.buf, buf...)
	c.len -= len(buf)
	return nil, false
}

type findContext struct {
	sub []byte
	buf []byte
}

func (c *findContext) find(buf []byte) (remain []byte, ok bool) {
	if len(c.buf)+len(buf) < len(c.sub) {
		c.buf = append(c.buf, buf...)
		return nil, false
	}
	if len(c.buf) > 0 {
		if len(buf) <= len(c.sub) {
			c.buf = append(c.buf, buf...)
			if i := bytes.Index(c.buf, c.sub); i >= 0 {
				c.buf = c.buf[i+len(c.sub):]
				return nil, true
			}
			c.buf = c.buf[len(c.buf)-len(c.sub)+1:]
			return nil, false
		}
		c.buf = append(c.buf, buf[:len(c.sub)]...)
		if i := bytes.Index(c.buf, c.sub); i >= 0 {
			c.buf = c.buf[i+len(c.sub):]
			return buf[len(c.sub):], true
		}
		c.buf = c.buf[:0]
	}
	if i := bytes.Index(buf, c.sub); i > 0 {
		return buf[i+len(c.sub):], true
	}
	c.buf = buf[len(buf)-len(c.sub)+1:]
	return nil, false
}

type Parser struct {
	layer   *Layer
	buf     []byte
	readLen int
	inBuf   []byte
	outBuf  []byte
	off     int
	state   int
	fd      findContext
	rd      readContext
}

func (p *Parser) find(buf []byte) (remain []byte, ok bool) {
	if len(p.inBuf)+len(buf) < len(startCodePrefix) {
		p.inBuf = append(p.inBuf, buf...)
		return nil, false
	}
	if len(p.inBuf) > 0 {
		if len(buf) <= len(startCodePrefix) {
			p.inBuf = append(p.inBuf, buf...)
			if i := bytes.Index(p.inBuf, startCodePrefix); i >= 0 {
				p.inBuf = p.inBuf[i+len(startCodePrefix):]
				return nil, true
			}
			p.inBuf = p.inBuf[len(p.inBuf)-len(startCodePrefix)+1:]
			return nil, false
		}
		p.inBuf = append(p.inBuf, buf[:len(startCodePrefix)]...)
		if i := bytes.Index(p.inBuf, startCodePrefix); i >= 0 {
			p.inBuf = p.inBuf[i+len(startCodePrefix):]
			return buf[len(startCodePrefix):], true
		}
		p.inBuf = p.inBuf[:0]
	}
	if i := bytes.Index(buf, startCodePrefix); i > 0 {
		return buf[i+len(startCodePrefix):], true
	}
	p.inBuf = buf[len(buf)-len(startCodePrefix)+1:]
	return nil, false
}

func (p *Parser) read(buf []byte) (remain []byte, ok bool) {
	if len(p.inBuf) >= p.readLen {
		remain, p.outBuf = buf, p.inBuf[:p.readLen]
		p.inBuf = p.inBuf[p.readLen:]
		p.readLen = 0
		return
	}
	cut := p.readLen - len(p.inBuf)
	if len(buf) < cut {
		p.inBuf = append(p.inBuf, buf...)
		p.readLen -= len(buf)
		return nil, false
	}
	if len(p.inBuf) == 0 {
		p.outBuf = buf[:cut]
	} else {
		p.outBuf = append(p.outBuf, buf[:cut]...)
	}
	p.readLen = 0
	remain = buf[cut:]
	p.buf = p.buf[:0]
	return
}

func (p *Parser) parseStartCodeSuffix(suffix byte) {
	switch suffix {
	case PackStartCode & 0x000000ff:
		p.state = stateParsePackHeader
	case SystemHeaderStartCode & 0x000000ff:
		p.state = stateParseSystemHeader
	//case StreamIdProgramStreamMap:
	//	p.state = stateParsePSM
	default:
		p.state = stateParsePES
	}
}

func (p *Parser) Parse(buf []byte) (remain []byte, err error) {
	var ok bool
	layer := p.layer
	for len(buf) > 0 {
		switch p.state {
		case stateParseStartCodePrefix:
			buf, ok = p.find(buf)
			if !ok {
				return nil, nil
			}
			//if len(p.buf) == 0 && len(buf) >= 4 {
			//	// break and find start code prefix
			//} else if len(buf)+len(p.buf) >= 4 {
			//	if len(buf) < 4 {
			//		cut := len(buf)
			//		bpp := append(p.buf, buf[:cut]...)
			//		if i := bytes.Index(bpp, startCodePrefix); i > 0 {
			//			p.buf = bpp[i+len(startCodePrefix):]
			//			p.state = stateParseStartCodeSuffix
			//			return nil, nil
			//		}
			//		p.buf = bpp[len(bpp)-2:]
			//		return nil, nil
			//	}
			//	bpp := append(p.buf, buf[:3]...)
			//	if i := bytes.Index(bpp, startCodePrefix); i > 0 {
			//		p.buf = bpp[i+len(startCodePrefix):]
			//		buf = buf[3:]
			//		p.parseStartCodeSuffix(buf[0])
			//	} else {
			//		p.buf = p.buf[:0]
			//	}
			//} else {
			//	p.buf = append(p.buf, buf...)
			//	return nil, nil
			//}
			//if i := bytes.Index(buf, startCodePrefix); i > 0 {
			//	p.buf = p.buf[:0]
			//	buf = buf[i+len(startCodePrefix):]
			//	if len(buf) == 0 {
			//		p.state = stateParseStartCodeSuffix
			//		return nil, nil
			//	}
			//	p.parseStartCodeSuffix(buf[0])
			//}
		case stateParseStartCodeSuffix:
			p.parseStartCodeSuffix(buf[0])
		case stateParsePackHeader:
			var bp []byte
			psh := &layer.PSH
			p.rd.read(buf)
			if buf, bp = p.read(buf, 11); bp == nil {
				return buf, nil
			}
			psh.packStartCode = PackStartCode
			copy(psh.bytes10[:], bp[1:])
			p.state = stateParseStartCodePrefix
		case stateParseSystemHeader:
			var bp []byte
			sys := &layer.SYS
			if buf, bp = p.read(buf, 9); bp == nil {
				return buf, nil
			}
			headerLength := binary.BigEndian.Uint16(bp[1:])
			if headerLength < 6 || (headerLength-6)%3 != 0 {
				return buf, SystemHeaderLengthInvalidError
			}
			sys.systemHeaderStartCode = SystemHeaderStartCode
			sys.headerLength = headerLength
			copy(sys.bytes6[:], bp[3:])
			p.state = stateParseSystemHeaderStreamInfos
		case stateParseSystemHeaderStreamInfos:
			var bp []byte
			sys := &layer.SYS
			size := int(sys.headerLength - 6)
			length := size / 3
			if buf, bp = p.read(buf, size); bp == nil {
				return buf, nil
			}
			sys.initStreamInfos(length)
			for i := 0; i < length; i++ {
				sys.streamInfos[i].streamId = bp[0]
				sys.streamInfos[i].bytes2[0] = bp[1]
				sys.streamInfos[i].bytes2[1] = bp[2]
				bp = bp[3:]
			}
			p.state = stateParseStartCodePrefix
		case stateParsePSM:
			var bp []byte
			psm := &layer.PSM
			if buf, bp = p.read(buf, 3); bp == nil {
				return buf, nil
			}
			psm.packetStartCodePrefix = startCodePrefixArray
			psm.streamId = bp[0]
			psm.pesPacketLength = binary.BigEndian.Uint16(bp[1:])
			p.state = stateParsePSM_PESHeader
		case stateParsePSM_PESHeader:

		case stateParsePES:
			//var bp []byte

		}
	}
	return
}
