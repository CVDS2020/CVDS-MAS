package rtsp2

import (
	"encoding/binary"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/cvds-mas/parser"
	"gitee.com/sy_183/rtp/rtp"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultMaxLineSize      = 2048
	DefaultMaxHeaderSize    = 8192
	DefaultMaxContentLength = 65536
)

const (
	parserStateParseStartCode = iota
	parserStateParseStartLine
	parserStateParseHeader
	parserStateParseBody
	parserStateParseRtpPrefix
	parserStateParseRtpPacket
)

const (
	PacketTypeUnknown = iota
	PacketTypeRequest
	PacketTypeResponse
	PacketTypeRtp
)

var (
	RtspStartLineFormatError = errors.New("RTSP起始行格式错误")
	RtspVersionError         = errors.New("RTSP版本错误")
	RtspStatusCodeError      = errors.New("RTSP状态码错误")
	RtspMethodError          = errors.New("无效的RTSP方法")
	RtspHeaderFormatError    = errors.New("RTSP头部格式错误")
)

type HeaderParser struct {
	MaxLineSize   int
	MaxHeaderSize int

	key, value string
	curHeader  textproto.MIMEHeader
	headerSize int

	lineParser parser.LineParser
	header     textproto.MIMEHeader
}

func (p *HeaderParser) Parse(data []byte) (ok bool, remain []byte, err error) {
	defer func() {
		if ok || err != nil {
			p.reset()
		}
	}()

	remain = data
	for {
		if ok, err = p.lineParser.ParseP(remain, &remain); !ok {
			return
		}
		line := p.lineParser.Line()

		if len(line) == 0 {
			if p.key != "" {
				if p.curHeader == nil {
					p.curHeader = make(textproto.MIMEHeader)
				}
				p.curHeader[p.key] = append(p.curHeader[p.key], p.value)
			}
			p.header = p.curHeader
			return true, remain, nil
		}

		if p.MaxHeaderSize <= 0 {
			p.MaxHeaderSize = DefaultMaxHeaderSize
		}
		if p.headerSize += len(line); p.headerSize > p.MaxHeaderSize {
			return false, remain, errors.NewSizeOutOfRange("RTSP头部", 0, int64(p.MaxLineSize), int64(p.headerSize), false)
		}

		if line[0] == ' ' || line[0] == '\t' {
			if p.key == "" {
				return false, remain, RtspHeaderFormatError
			}
			p.value += " " + strings.Trim(line, " \t")
			continue
		}

		if p.key != "" {
			if p.curHeader == nil {
				p.curHeader = make(textproto.MIMEHeader)
			}
			p.curHeader[p.key] = append(p.curHeader[p.key], p.value)
		}

		if i := strings.IndexByte(line, ':'); i > 0 {
			p.key = textproto.CanonicalMIMEHeaderKey(line[:i])
			p.value = strings.Trim(line[i+1:], " \t")
		} else {
			return false, remain, RtspHeaderFormatError
		}
	}
}

func (p *HeaderParser) ParseP(data []byte, remainP *[]byte) (ok bool, err error) {
	ok, *remainP, err = p.Parse(data)
	return
}

func (p *HeaderParser) Header() textproto.MIMEHeader {
	return p.header
}

func (p *HeaderParser) reset() {
	p.key, p.value = "", ""
	p.curHeader = nil
	p.headerSize = 0
}

func (p *HeaderParser) Reset() {
	p.reset()
	p.header = nil
	p.lineParser.Reset()
}

type Parser struct {
	maxContentLength int64

	packetType int

	message  *Message
	request  *Request
	response *Response

	rtpStreamId uint8
	rtpLength   uint16
	rtpPacket   *rtp.IncomingPacket

	rtpPacketPool pool.Pool[*rtp.IncomingPacket]

	lineParser   parser.LineParser
	headerParser HeaderParser
	needParser   parser.NeedParser
	state        int
}

var isTokenTable = [127]bool{
	'!':  true,
	'#':  true,
	'$':  true,
	'%':  true,
	'&':  true,
	'\'': true,
	'*':  true,
	'+':  true,
	'-':  true,
	'.':  true,
	'0':  true,
	'1':  true,
	'2':  true,
	'3':  true,
	'4':  true,
	'5':  true,
	'6':  true,
	'7':  true,
	'8':  true,
	'9':  true,
	'A':  true,
	'B':  true,
	'C':  true,
	'D':  true,
	'E':  true,
	'F':  true,
	'G':  true,
	'H':  true,
	'I':  true,
	'J':  true,
	'K':  true,
	'L':  true,
	'M':  true,
	'N':  true,
	'O':  true,
	'P':  true,
	'Q':  true,
	'R':  true,
	'S':  true,
	'T':  true,
	'U':  true,
	'W':  true,
	'V':  true,
	'X':  true,
	'Y':  true,
	'Z':  true,
	'^':  true,
	'_':  true,
	'`':  true,
	'a':  true,
	'b':  true,
	'c':  true,
	'd':  true,
	'e':  true,
	'f':  true,
	'g':  true,
	'h':  true,
	'i':  true,
	'j':  true,
	'k':  true,
	'l':  true,
	'm':  true,
	'n':  true,
	'o':  true,
	'p':  true,
	'q':  true,
	'r':  true,
	's':  true,
	't':  true,
	'u':  true,
	'v':  true,
	'w':  true,
	'x':  true,
	'y':  true,
	'z':  true,
	'|':  true,
	'~':  true,
}

func IsTokenRune(r rune) bool {
	i := int(r)
	return i < len(isTokenTable) && isTokenTable[i]
}

func isNotToken(r rune) bool {
	return !IsTokenRune(r)
}

func parseRTSPVersion(proto string) (protoMajor, protoMinor int, ok bool) {
	switch proto {
	case "RTSP/1.0":
		return 1, 0, true
	}
	if !strings.HasPrefix(proto, "RTSP/") {
		return 0, 0, false
	}
	if len(proto) != len("HTTP/X.Y") {
		return 0, 0, false
	}
	if proto[6] != '.' {
		return 0, 0, false
	}
	maj, err := strconv.ParseUint(proto[5:6], 10, 0)
	if err != nil {
		return 0, 0, false
	}
	min, err := strconv.ParseUint(proto[7:8], 10, 0)
	if err != nil {
		return 0, 0, false
	}
	return int(maj), int(min), true
}

func validMethod(method string) bool {
	return len(method) > 0 && strings.IndexFunc(method, isNotToken) == -1
}

func splitStartLine(line string) (line1, line2, line3 string, ok bool) {
	line1, rest, ok1 := strings.Cut(line, " ")
	line2, line3, ok2 := strings.Cut(rest, " ")
	if !ok1 || !ok2 {
		return "", "", "", false
	}
	return line1, line2, line3, true
}

func NewParser(maxLineSize, maxHeaderSize int, maxContentLength int64) *Parser {
	if maxLineSize <= 0 {
		maxLineSize = DefaultMaxLineSize
	}
	if maxHeaderSize <= 0 {
		maxHeaderSize = DefaultMaxHeaderSize
	}
	if maxContentLength <= 0 {
		maxContentLength = DefaultMaxContentLength
	}
	return &Parser{
		maxContentLength: maxContentLength,
		rtpPacketPool:    pool.ProvideSyncPool(rtp.ProvideIncomingPacket),
		lineParser: parser.LineParser{
			MaxLineSize: maxLineSize,
		},
		headerParser: HeaderParser{
			MaxHeaderSize: maxHeaderSize,
		},
	}
}

func (p *Parser) parseStartCode(data []byte, remainP *[]byte) bool {
	for len(data) > 0 {
		switch data[0] {
		case '\r', '\n':
			data = data[1:]
			continue
		case '$':
			// RTP over RTSP
			p.state = parserStateParseRtpPrefix
			p.packetType = PacketTypeRtp
			p.needParser.SetNeed(4)
		default:
			// RTSP 控制信令
			p.state = parserStateParseStartLine
		}
		*remainP = data
		return true
	}
	*remainP = nil
	return false
}

func (p *Parser) parseStartLine(data []byte, remainP *[]byte) (ok bool, err error) {
	defer func() {
		if err != nil {
			err = WrapRtspError(err)
			p.resetMessage()
			p.state = parserStateParseStartCode
		} else if ok {
			p.state = parserStateParseHeader
		}
	}()

	if ok, err = p.lineParser.ParseP(data, remainP); ok {
		line1, line2, line3, ok := splitStartLine(p.lineParser.Line())
		if !ok {
			return false, RtspStartLineFormatError
		}

		if strings.HasPrefix(line1, "RTSP") {
			p.packetType = PacketTypeResponse
			p.response = new(Response)
			p.message = &p.response.Message
			resp := p.response

			resp.proto = line1
			resp.protoMajor, resp.protoMinor, ok = parseRTSPVersion(resp.proto)
			if !ok {
				return false, RtspVersionError
			}

			if len(line2) != 3 {
				return false, RtspStatusCodeError
			}
			resp.statusCode, err = strconv.Atoi(line2)
			if err != nil || resp.statusCode < 0 {
				return false, RtspStatusCodeError
			}

			resp.reasonPhrase = line3
			return true, nil
		}
		p.packetType = PacketTypeRequest
		p.request = new(Request)
		p.message = &p.request.Message
		req := p.request

		req.method = line1
		if !validMethod(req.method) {
			return false, RtspMethodError
		}

		req.requestURI = line2
		if req.url, err = url.ParseRequestURI(req.requestURI); err != nil {
			return false, err
		}

		req.proto = line3
		if req.protoMajor, req.protoMinor, ok = parseRTSPVersion(req.proto); !ok {
			return false, RtspVersionError
		}
		return true, nil
	}
	return
}

func (p *Parser) parseHeaderFields() (contentLength int64, err error) {
	if header := p.message.header; header != nil {
		if clHeader := header["Content-Length"]; len(clHeader) > 0 {
			contentLength, err := strconv.ParseInt(clHeader[0], 10, 64)
			if err != nil {
				return 0, err
			} else if contentLength < 0 || contentLength > p.maxContentLength {
				return 0, errors.NewSizeOutOfRange("RTSP正文", 0, p.maxContentLength, contentLength, true)
			}
			p.message.contentLength = contentLength
		}
		if ctHeader := header["Content-Type"]; len(ctHeader) > 0 {
			p.message.contentType = ctHeader[0]
		}
		if cSeqHeader := header["Cseq"]; len(cSeqHeader) > 0 {
			cSeq, err := strconv.ParseInt(cSeqHeader[0], 10, 64)
			if err != nil {
				return p.message.contentLength, err
			} else if cSeq < 0 {
				return p.message.contentLength, errors.New("解析RTSP CSeq错误")
			}
			p.message.cSeq = int(cSeq)
		}
		if session := header["Session"]; len(session) > 0 {
			p.message.session = session[0]
		}
	}
	return 0, nil
}

func (p *Parser) parseHeader(data []byte, remainP *[]byte) (ok bool, err error) {
	var contentLength int64
	defer func() {
		if err != nil {
			err = WrapRtspError(err)
			p.resetMessage()
			if contentLength > 0 {
				p.state = parserStateParseBody
				p.needParser.SetNeed(int(contentLength))
			} else {
				p.state = parserStateParseStartCode
			}
		} else if ok {
			if p.message.contentLength > 0 {
				p.state = parserStateParseBody
				p.needParser.SetNeed(int(contentLength))
			} else {
				p.state = parserStateParseStartCode
			}
		}
	}()

	if ok, *remainP, err = p.headerParser.Parse(data); ok {
		if header := p.headerParser.Header(); header != nil {
			p.message.header = header
			if contentLength, err = p.parseHeaderFields(); err != nil {
				return false, err
			}
		}
	}
	return
}

func (p *Parser) parseBody(data []byte, remainP *[]byte) (ok bool) {
	if p.message == nil {
		if p.needParser.SkipP(data, remainP) {
			p.state = parserStateParseStartCode
		}
		return false
	}
	if *remainP, ok = p.needParser.Parse(data); ok {
		p.message.body = p.needParser.Get()
		p.state = parserStateParseStartCode
	}
	return
}

func (p *Parser) parseRtpPrefix(data []byte, remainP *[]byte) (ok bool, err error) {
	defer func() {
		if err != nil {
			err = WrapRtpError(err)
			p.resetRtp()
		}
	}()

	if *remainP, ok = p.needParser.Parse(data); ok {
		prefix := p.needParser.Merge()
		p.rtpStreamId = prefix[1]
		p.rtpLength = binary.BigEndian.Uint16(prefix[2:])

		p.state = parserStateParseRtpPacket
		p.needParser.SetNeed(int(p.rtpLength))

		if p.rtpPacket != nil {
			p.rtpPacket.Release()
			p.rtpPacket = p.rtpPacketPool.Get()
			if p.rtpPacket == nil {
				return false, pool.NewAllocError("RTP包")
			}
			p.rtpPacket.Use()
		}
	}
	return
}

func (p *Parser) parseRtpPacket(data []byte, remainP *[]byte) (ok bool, err error) {
	defer func() {
		if err != nil {
			err = WrapRtpError(err)
			p.resetRtp()
			p.state = parserStateParseStartCode
		} else if ok {
			p.state = parserStateParseStartCode
		}
	}()
	if p.rtpPacket == nil {
		// 即使RTP包申请失败，也要跳过RTP包的长度
		if p.needParser.SkipP(data, remainP) {
			p.state = parserStateParseStartCode
		}
		return
	}
	if p.needParser.ParseP(data, remainP) {
		if err = p.rtpPacket.UnmarshalChunks(p.needParser.Get()); err != nil {
			p.rtpPacket.Release()
			p.rtpPacket = nil
			return
		}
	}
	return
}

func (p *Parser) Parse(data []byte) (ok bool, remain []byte, err error) {
	remain = data
	for len(remain) > 0 {
		switch p.state {
		case parserStateParseStartCode:
			p.parseStartCode(remain, &remain)

		case parserStateParseStartLine:
			if _, err := p.parseStartLine(remain, &remain); err != nil {
				return false, remain, err
			}
		case parserStateParseHeader:
			if ok, err := p.parseHeader(remain, &remain); err != nil {
				return false, remain, err
			} else if ok {
				if p.state == parserStateParseStartCode {
					return true, remain, nil
				}
			}
		case parserStateParseBody:
			if p.parseBody(remain, &remain) {
				return true, remain, nil
			}

		case parserStateParseRtpPrefix:
			if _, err := p.parseRtpPrefix(remain, &remain); err != nil {
				return false, remain, err
			}
		case parserStateParseRtpPacket:
			if ok, err := p.parseRtpPacket(remain, &remain); err != nil {
				return false, remain, err
			} else if ok {
				return true, remain, nil
			}
		}
	}
	return
}

func (p *Parser) ParseP(data []byte, remainP *[]byte) (ok bool, err error) {
	ok, *remainP, err = p.Parse(data)
	return
}

func (p *Parser) PacketType() int {
	return p.packetType
}

func (p *Parser) Request() *Request {
	return p.request
}

func (p *Parser) Response() *Response {
	return p.response
}

func (p *Parser) RtpPacket() (packet *rtp.IncomingPacket, streamId uint8, length uint16) {
	if p.rtpPacket == nil {
		return nil, 0, 0
	}
	return p.rtpPacket.Use(), p.rtpStreamId, p.rtpLength
}

func (p *Parser) resetMessage() {
	p.packetType = PacketTypeUnknown
	p.message = nil
	p.request = nil
	p.response = nil
}

func (p *Parser) resetRtp() {
	p.packetType = PacketTypeUnknown
	if p.rtpPacket != nil {
		p.rtpPacket.Release()
		p.rtpPacket = nil
	}
	p.rtpStreamId = 0
	p.rtpLength = 0
}

func (p *Parser) Reset() {
	p.resetMessage()
	p.resetRtp()
	p.lineParser.Reset()
	p.headerParser.Reset()
	p.needParser.Reset()
}
