package parser

import "bytes"

type NeedParser struct {
	cache  [][]byte
	size   int
	need   int
	chunks [][]byte
}

func (p *NeedParser) Need() int {
	return p.need
}

func (p *NeedParser) SetNeed(need int) {
	p.need = need
}

func (p *NeedParser) Skip(data []byte) (remain []byte, ok bool) {
	if p.size+len(data) >= p.need {
		need := p.need - p.size
		p.reset()
		return data[need:], true
	}
	p.size += len(data)
	return nil, false
}

func (p *NeedParser) SkipP(data []byte, remainP *[]byte) (ok bool) {
	*remainP, ok = p.Skip(data)
	return
}

func (p *NeedParser) Parse(data []byte) (remain []byte, ok bool) {
	if p.size+len(data) >= p.need {
		need := p.need - p.size
		p.cache = append(p.cache, data[:need])
		p.chunks = append([][]byte(nil), p.cache...)
		p.reset()
		return data[need:], true
	}
	p.cache = append(p.cache, data)
	p.size += len(data)
	return nil, false
}

func (p *NeedParser) ParseP(data []byte, remainP *[]byte) (ok bool) {
	*remainP, ok = p.Parse(data)
	return
}

func (p *NeedParser) Get() [][]byte {
	return p.chunks
}

func (p *NeedParser) Merge() []byte {
	switch len(p.chunks) {
	case 0:
		return nil
	case 1:
		return p.chunks[0]
	default:
		return bytes.Join(p.chunks, nil)
	}
}

func (p *NeedParser) reset() {
	p.cache = p.cache[:0]
	p.size = 0
}

func (p *NeedParser) Reset() {
	p.reset()
	p.chunks = nil
}
