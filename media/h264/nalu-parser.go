package h264

import (
	"bytes"
	"fmt"
	"gitee.com/sy_183/common/uns"
	"gitee.com/sy_183/cvds-mas/parser"
)

var (
	h264StartCode3 = []byte{0, 0, 1}
	h264StartCode4 = []byte{0, 0, 0, 1}
)

type AnnexBInPrefixNALUParser struct {
	curNaluHeader NALUHeader
	cache         [][]byte
	cacheSize     int
	skip          bool

	naluHeader   NALUHeader
	naluBody     [][]byte
	naluBodySize int

	needParser parser.NeedParser
}

func NewAnnexBInPrefixNALUParser(initCache int) *AnnexBInPrefixNALUParser {
	p := &AnnexBInPrefixNALUParser{
		skip: true,
	}
	if initCache > 0 {
		p.cache = make([][]byte, 0, initCache)
	}
	return p
}

func (p *AnnexBInPrefixNALUParser) parsePrefix(prefix []byte) (naluOffset int) {
	if bytes.HasPrefix(prefix, h264StartCode3) {
		return 3
	} else if bytes.HasPrefix(prefix, h264StartCode4) {
		return 4
	}
	return 0
}

func (p *AnnexBInPrefixNALUParser) complete() {
	p.naluHeader, p.naluBody, p.naluBodySize = p.curNaluHeader, append([][]byte(nil), p.cache...), p.cacheSize
	p.curNaluHeader, p.cache, p.cacheSize = 0, p.cache[:0], 0
}

func (p *AnnexBInPrefixNALUParser) checkHeader(header NALUHeader) error {
	if naluType := header.Type(); naluType < NALUTypeSlice || naluType > NALUTypeFILL {
		p.skip = true
		return fmt.Errorf("无效的H264 NALU类型(%d)", naluType)
	}
	return nil
}

func (p *AnnexBInPrefixNALUParser) appendCache(chunks ...[]byte) {
	p.cache = append(p.cache, chunks...)
	for _, chunk := range chunks {
		p.cacheSize += len(chunk)
	}
}

func (p *AnnexBInPrefixNALUParser) Parse(data []byte, isPrefix bool) (ok bool, err error) {
	if isPrefix {
		p.needParser.SetNeed(5)
		var remain []byte
		if p.needParser.ParseP(data, &remain) {
			prefix := p.needParser.Merge()
			if naluOffset := p.parsePrefix(prefix); naluOffset != 0 {
				if !p.skip {
					// 之前的数据已经解析成NALU
					p.complete()
					ok = true
				} else {
					// 第一次解析，或是之前NALU解析错误
					p.skip = false
				}
				naluHeader := NALUHeader(prefix[naluOffset])
				if err = p.checkHeader(naluHeader); err != nil {
					return ok, err
				}
				p.curNaluHeader = naluHeader
				parsed := len(data) - len(remain)
				if naluOffset == 3 {
					p.appendCache(data[parsed-1:])
				} else if len(remain) > 0 {
					p.appendCache(remain)
				}
				return ok, nil
			}
			if !p.skip {
				if uns.SlicePointer(prefix) == uns.SlicePointer(data) {
					p.appendCache(data)
				} else {
					p.appendCache(prefix, remain)
				}
			}
		}
		return false, nil
	}
	if !p.skip {
		p.appendCache(data)
	}
	return false, nil
}

func (p *AnnexBInPrefixNALUParser) NALU() (header NALUHeader, body [][]byte, bodySize int) {
	return p.naluHeader, p.naluBody, p.naluBodySize
}

func (p *AnnexBInPrefixNALUParser) Complete() bool {
	if !p.skip {
		p.complete()
		p.skip = true
		return true
	}
	return false
}

func (p *AnnexBInPrefixNALUParser) Reset() {
	p.Complete()
	p.naluHeader = 0
	p.naluBody = nil
	p.naluBodySize = 0
	p.needParser.Reset()
}
