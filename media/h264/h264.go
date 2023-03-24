package h264

const (
	NALUTypeSlice    = 1
	NALUTypeDPA      = 2
	NALUTypeDPB      = 3
	NALUTypeDBC      = 4
	NALUTypeIDR      = 5
	NALUTypeSEI      = 6
	NALUTypeSPS      = 7
	NALUTypePPS      = 8
	NALUTypeAUD      = 9
	NALUTypeEOSEQ    = 10
	NALUTypeEOSTRREM = 11
	NALUTypeFILL     = 12

	NALUTypeSTAPA  = 24
	NALUTypeSTAPB  = 25
	NALUTypeMTAP16 = 26
	NALUTypeMTAP24 = 27
	NALUTypeFUA    = 28
	NALUTypeFUB    = 28
)

type NALUHeader byte

func (h NALUHeader) Type() uint8 {
	return uint8(h & 0b00011111)
}

func (h *NALUHeader) SetType(typ uint8) {
	*h |= NALUHeader(typ & 0b00011111)
}

func (h NALUHeader) NRI() uint8 {
	return (uint8(h) >> 5) & 0b11
}

func (h *NALUHeader) SetNRI(nri uint8) {
	*h |= NALUHeader((nri & 0b11) << 5)
}

type FUHeader byte

func (h FUHeader) IsStart() bool {
	return h>>7 == 1
}

func (h *FUHeader) SetStart(set bool) {
	if set {
		*h |= 0b10000000
	} else {
		*h &= 0b01111111
	}
}

func (h FUHeader) IsEnd() bool {
	return (h>>6)&0b01 == 1
}

func (h *FUHeader) SetEnd(set bool) {
	if set {
		*h |= 0b01000000
	} else {
		*h &= 0b10111111
	}
}

func (h FUHeader) Type() uint8 {
	return uint8(h & 0b00011111)
}

func (h *FUHeader) SetType(typ uint8) {
	*h |= FUHeader(typ & 0b00011111)
}

type SubNALU struct {
	size     uint16
	dond     uint8
	tsOffset uint32
	header   uint8
	payload  [][]byte
}

type NALU struct {
	header   uint8
	fuHeader uint8
	don      uint16
	uintSize uint16
	payload  [][]byte
	sub      []SubNALU
}

func (n *NALU) Type() uint8 {
	return n.header & 0b00011111
}

func (n *NALU) SetType(typ uint8) {
	n.header |= typ & 0b00011111
}

func (n *NALU) NRI() uint8 {
	return (n.header >> 5) & 0b11
}

func (n *NALU) SetNRI(nri uint8) {
	n.header |= (nri & 0b11) << 5
}

func (n *NALU) FUStart() bool {
	return n.fuHeader>>7 == 1
}

func (n *NALU) SetFUStart(set bool) {
	if set {
		n.fuHeader |= 0b10000000
	} else {
		n.fuHeader &= 0b01111111
	}
}

func (n *NALU) FUEnd() bool {
	return (n.fuHeader>>6)&0b01 == 1
}

func (n *NALU) SetFUEnd(set bool) {
	if set {
		n.fuHeader |= 0b01000000
	} else {
		n.fuHeader &= 0b10111111
	}
}

func (n *NALU) FUType() uint8 {
	return n.fuHeader & 0b00011111
}

func (n *NALU) SetFUType(typ uint8) {
	n.fuHeader |= typ & 0b00011111
}

func (n *NALU) DON() uint16 {
	return n.don
}

func (n *NALU) SetDON(don uint16) {
	n.don = don
}

func (n *NALU) UnitSize() uint16 {
	return n.uintSize
}

func (n *NALU) Payload() [][]byte {
	return n.payload
}

func (n *NALU) SetPayload(payload [][]byte, size uint16) {
	switch n.Type() {
	case NALUTypeSTAPA,
		NALUTypeSTAPB,
		NALUTypeMTAP16,
		NALUTypeMTAP24:
		return
	default:
		n.payload = payload
		n.uintSize = size
	}
}

func (n *NALU) Sub() []SubNALU {
	return n.sub
}
