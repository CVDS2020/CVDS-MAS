package h264

import (
	"fmt"
	"testing"
)

func printNALU(header NALUHeader, body NALUBody) {
	fmt.Printf("NALU Type: %d, NALU Size: %d\n", header.Type(), body.Size())
}

func parse(p *AnnexBInPrefixNALUParser, data []byte, isPrefix bool) {
	ok, err := p.Parse(data, isPrefix)
	if ok {
		printNALU(p.NALU())
	}
	if err != nil {
		fmt.Println(err)
	}
}

func TestAnnexBInPrefixNALUParser(t *testing.T) {
	p := NewAnnexBInPrefixNALUParser(0)
	parse(p, []byte{0, 0, 0, 1, 67, 1, 2, 3, 4, 5, 6}, true)
	parse(p, []byte{7, 8, 9, 10}, false)
	parse(p, []byte{0, 0, 1, 9, 9, 10}, true)
	parse(p, []byte{11, 12, 13, 14, 15, 16}, true)
	parse(p, []byte{15, 16, 17, 18, 19, 20, 21}, true)
	parse(p, []byte{0, 0, 1, 9, 0}, true)
	parse(p, []byte{0, 0, 1, 9, 0}, true)
	parse(p, []byte{0, 0, 1, 9, 0}, true)
	parse(p, []byte{0, 0, 1, 9, 0}, true)
	if p.Complete() {
		printNALU(p.NALU())
	}
}
