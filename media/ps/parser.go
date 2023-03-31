package ps

import (
	"encoding/binary"
	"errors"
	"gitee.com/sy_183/cvds-mas/parser"
)

const (
	parserStateFindStartCode = iota
	parserStateParsePSH
	parserStateParsePSHStuffing
	parserStateParseSYS
	parserStateParseSYSStreamInfos
	parserStateParsePSMPacketLength
	parserStateParsePSMHeader
	parserStateParsePESPacketLength
	parserStateParseHeaderDataLength
	parserStateParsePESHeader
	parserStateParsePacketData
)

var (
	SysHeaderLengthError              = errors.New("PS SYS 头部长度错误")
	PSMLengthError                    = errors.New("PS PSM 包长度错误")
	PSMElementaryStreamMapLengthError = errors.New("PS PSM ElementaryStreamMap 长度错误")
	PESLengthError                    = errors.New("PS PES 包长度错误")
	PESDataLengthError                = errors.New("PS PEM HeaderData 长度错误")
)

type Parser struct {
	psh *PSH
	sys *SYS
	psm *PSM
	pes *PES

	startCodeFinder StartCodeFinder
	startCode       [4]byte

	needParser parser.NeedParser

	state int
}

func (p *Parser) Reset() {
	if p.psh != nil {
		p.psh.Clear()
	}
	if p.sys != nil {
		p.sys.Clear()
	}
	if p.psm != nil {
		p.psm.Clear()
	}
	if p.pes != nil {
		p.pes.Clear()
	}
	p.needParser.Reset()
	p.state = parserStateFindStartCode
	p.startCodeFinder.Reset()
	p.startCode = [4]byte{}
}

func (p *Parser) SetPSH(psh *PSH) (old *PSH) {
	old = p.psh
	p.psh = psh
	return
}

func (p *Parser) SetSYS(sys *SYS) (old *SYS) {
	old = p.sys
	p.sys = sys
	return
}

func (p *Parser) SetPSM(psm *PSM) (old *PSM) {
	old = p.psm
	p.psm = psm
	return
}

func (p *Parser) SetPES(pes *PES) (old *PES) {
	old = p.pes
	p.pes = pes
	return
}

func (p *Parser) findStartCode(data []byte, remainP *[]byte) (ok bool) {
	p.startCode, *remainP, ok = p.startCodeFinder.Find(data)
	if ok {
		switch p.startCode[3] {
		case 0xBA:
			p.psh.startCode = p.startCode
			p.state = parserStateParsePSH
			p.needParser.SetNeed(10)
		case 0xBB:
			p.sys.startCode = p.startCode
			p.state = parserStateParseSYS
			p.needParser.SetNeed(8)
		case StreamIdProgramStreamMap:
			p.psm.startCode = p.startCode
			p.state = parserStateParsePSMPacketLength
			p.needParser.SetNeed(2)
		default:
			p.pes.startCode = p.startCode
			p.state = parserStateParsePESPacketLength
			p.needParser.SetNeed(2)
		}
	}
	return
}

func (p *Parser) parsePSH(data []byte, remainP *[]byte) bool {
	if p.needParser.ParseP(data, remainP) {
		copy(p.psh.bytes10[:], p.needParser.Merge())
		p.state = parserStateParsePSHStuffing
		p.needParser.SetNeed(int(p.psh.PackStuffingLength()))
		return true
	}
	return false
}

func (p *Parser) parsePSHStuffing(data []byte, remainP *[]byte) bool {
	if p.needParser.SkipP(data, remainP) {
		p.state = parserStateFindStartCode
		return true
	}
	return false
}

func (p *Parser) parseSYS(data []byte, remainP *[]byte) (ok bool, err error) {
	if p.needParser.ParseP(data, remainP) {
		sys := p.sys
		raw := p.needParser.Merge()
		sys.headerLength = binary.BigEndian.Uint16(raw)
		siDataSize := sys.headerLength - 6
		if siDataSize < 0 || siDataSize%3 != 0 {
			return false, SysHeaderLengthError
		}
		copy(sys.bytes6[:], raw[2:])
		p.state = parserStateParseSYSStreamInfos
		p.needParser.SetNeed(int(siDataSize))
		return true, nil
	}
	return false, nil
}

func (p *Parser) parseSYSStreamInfos(data []byte, remainP *[]byte) bool {
	if p.needParser.ParseP(data, remainP) {
		sys := p.sys
		raw := p.needParser.Merge()
		siLength := p.needParser.Need() / 3
		for i := 0; i < siLength; i++ {
			sys.streamInfos = append(sys.streamInfos, SysStreamInfo{
				streamId: raw[0],
				bytes2:   [2]byte{raw[1], raw[2]},
			})
			raw = raw[3:]
		}
		p.state = parserStateFindStartCode
		return true
	}
	return false
}

func (p *Parser) parsePSMPacketLength(data []byte, remainP *[]byte) (ok bool, err error) {
	if p.needParser.ParseP(data, remainP) {
		psm := p.psm
		raw := p.needParser.Merge()
		psm.packetLength = binary.BigEndian.Uint16(raw)
		if psm.packetLength < 10 {
			return false, PSMLengthError
		}
		p.state = parserStateParsePSMHeader
		p.needParser.SetNeed(int(psm.packetLength))
		return true, nil
	}
	return false, nil
}

func (p *Parser) parsePSMHeader(data []byte, remainP *[]byte) (ok bool, err error) {
	if p.needParser.ParseP(data, remainP) {
		psm := p.psm
		raw := p.needParser.Merge()
		min := uint16(10)

		psm.programStreamInfoLength = binary.BigEndian.Uint16(raw[2:])
		if min += psm.programStreamInfoLength; psm.packetLength < min {
			return false, PSMLengthError
		}
		psm.bytes2[0], psm.bytes2[1] = raw[0], raw[1]
		raw = raw[4:]
		psm.descriptors = raw[:psm.programStreamInfoLength]
		raw = raw[psm.programStreamInfoLength:]

		psm.elementaryStreamMapLength = binary.BigEndian.Uint16(raw)
		raw = raw[2:]
		if min += psm.elementaryStreamMapLength; psm.packetLength < min {
			return false, PSMLengthError
		}

		esiRaw := raw[:psm.elementaryStreamMapLength]
		var esiMin uint16
		for len(esiRaw) >= 4 {
			if esiMin += 4; psm.elementaryStreamMapLength < esiMin {
				return false, PSMElementaryStreamMapLengthError
			}
			info := ElementaryStreamInfo{
				typ:        esiRaw[0],
				id:         esiRaw[1],
				infoLength: binary.BigEndian.Uint16(esiRaw[2:]),
			}
			esiRaw = esiRaw[4:]
			if esiMin += info.infoLength; psm.elementaryStreamMapLength < esiMin {
				return false, PSMElementaryStreamMapLengthError
			}
			info.descriptors = esiRaw[:info.infoLength]
			esiRaw = esiRaw[info.infoLength:]
			psm.elementaryStreamInfos = append(psm.elementaryStreamInfos, info)
		}
		if len(esiRaw) > 0 {
			return false, PSMElementaryStreamMapLengthError
		}
		raw = raw[psm.elementaryStreamMapLength:]

		psm.crc32 = binary.BigEndian.Uint32(raw)
		if raw = raw[4:]; len(raw) > 0 {
			return false, PSMLengthError
		}

		p.state = parserStateFindStartCode
		return true, nil
	}
	return false, nil
}

func (p *Parser) parsePESPacketLength(data []byte, remainP *[]byte) (ok bool, err error) {
	if p.needParser.ParseP(data, remainP) {
		pes := p.pes
		raw := p.needParser.Merge()
		pes.packetLength = binary.BigEndian.Uint16(raw)
		switch pes.StreamId() {
		case StreamIdProgramStreamMap,
			StreamIdPaddingStream,
			StreamIdPrivateStream2,
			StreamIdECMStream,
			StreamIdEMMStream,
			StreamIdProgramStreamDirectory,
			StreamIdISO13818_6DSMCCStream,
			StreamIdITUH2221TypeE:
			p.state = parserStateParsePacketData
			p.needParser.SetNeed(int(pes.packetLength))
		default:
			if pes.packetLength < 3 {
				return false, PESLengthError
			}
			p.state = parserStateParseHeaderDataLength
			p.needParser.SetNeed(3)
		}
		return true, nil
	}
	return false, nil
}

func (p *Parser) parsePESHeaderDataLength(data []byte, remainP *[]byte) (ok bool, err error) {
	if p.needParser.ParseP(data, remainP) {
		pes := p.pes
		raw := p.needParser.Merge()

		pes.headerDataLength = raw[2]
		if pes.packetLength < 3+uint16(pes.headerDataLength) {
			return false, PESLengthError
		}
		pes.bytes2[0], pes.bytes2[1] = raw[0], raw[1]
		p.state = parserStateParsePESHeader
		p.needParser.SetNeed(int(pes.headerDataLength))
		return true, nil
	}
	return false, nil
}

func (p *Parser) parsePESHeader(data []byte, remainP *[]byte) (ok bool, err error) {
	if p.needParser.ParseP(data, remainP) {
		pes := p.pes
		raw := p.needParser.Merge()

		var min uint8
		if pes.PTS_DTS_Flags() == 0b10 {
			if min += 5; pes.headerDataLength < min {
				return false, PESDataLengthError
			}
			copy(pes.pts[:], raw)
			raw = raw[5:]
		} else if pes.PTS_DTS_Flags() == 0b11 {
			if min += 10; pes.headerDataLength < min {
				return false, PESDataLengthError
			}
			copy(pes.pts[:], raw)
			copy(pes.dts[:], raw[5:])
			raw = raw[10:]
		}

		if pes.ESCRFlag() {
			if min += 6; pes.headerDataLength < min {
				return false, PESDataLengthError
			}
			copy(pes.escr[:], raw)
			raw = raw[6:]
		}

		if pes.ESRateFlag() {
			if min += 3; pes.headerDataLength < min {
				return false, PESDataLengthError
			}
			copy(pes.esRate[:], raw)
			raw = raw[:3]
		}

		if pes.DSMTrickModeFlag() {
			if min += 1; pes.headerDataLength < min {
				return false, PESDataLengthError
			}
			pes.dsmTrickMode = raw[0]
			raw = raw[1:]
		}

		if pes.AdditionalCopyInfoFlag() {
			if min += 1; pes.headerDataLength < min {
				return false, PESDataLengthError
			}
			pes.additionalCopyInfo = raw[0]
			raw = raw[1:]
		}

		if pes.CRCFlag() {
			if min += 2; pes.headerDataLength < min {
				return false, PESDataLengthError
			}
			pes.crc = binary.BigEndian.Uint16(raw)
			raw = raw[2:]
		}

		if pes.ExtensionFlag() {
			if min += 1; pes.headerDataLength < min {
				return false, PESDataLengthError
			}
			extension := &pes.extension
			extension.base = raw[0]
			raw = raw[1:]

			if pes.PrivateDataFlag() {
				if min += 16; pes.headerDataLength < min {
					return false, PESDataLengthError
				}
				copy(extension.privateData[:], raw)
				raw = raw[16:]
			}

			if pes.PackHeaderFieldFlag() {
				if min += 1; pes.headerDataLength < min {
					return false, PESDataLengthError
				}
				extension.packFieldLength = raw[0]
				raw = raw[1:]

				if min += extension.packFieldLength; pes.headerDataLength < min {
					return false, PESDataLengthError
				}
				extension.packHeader = raw[:extension.packFieldLength]
				raw = raw[extension.packFieldLength:]
			}

			if pes.ProgramPacketSequenceCounterFlag() {
				if min += 2; pes.headerDataLength < min {
					return false, PESDataLengthError
				}
				extension.programPacketSequenceCounter = raw[0]
				extension.mpeg1mpeg2Identifier = raw[1]
				raw = raw[2:]
			}

			if pes.P_STD_BufferFlag() {
				if min += 2; pes.headerDataLength < min {
					return false, PESDataLengthError
				}
				extension.pSTDBuffer[0] = raw[0]
				extension.pSTDBuffer[1] = raw[1]
				raw = raw[2:]
			}

			if pes.ExtensionFlag2() {
				if min += 1; pes.headerDataLength < min {
					return false, PESDataLengthError
				}
				extension.extensionFieldLength = raw[0]
				raw = raw[1:]

				if min += extension.extensionFieldLength; pes.headerDataLength < min {
					return false, PESDataLengthError
				}
				extension.extensionFieldReversed = raw[:extension.extensionFieldLength]
				raw = raw[extension.extensionFieldLength:]
			}
		}

		packetDataSize := pes.packetLength - uint16(pes.headerDataLength) - 3
		p.state = parserStateParsePacketData
		p.needParser.SetNeed(int(packetDataSize))
		return true, nil
	}
	return false, nil
}

func (p *Parser) parsePESPacketData(data []byte, remainP *[]byte) bool {
	if p.needParser.ParseP(data, remainP) {
		p.pes.packetData = PESPacketData{
			chunks: p.needParser.Get(),
			size:   p.needParser.Need(),
		}
		p.state = parserStateFindStartCode
		return true
	}
	return false
}

func (p *Parser) Parse(data []byte) (ok bool, remain []byte, err error) {
	defer func() {
		if err != nil {
			p.Reset()
		}
	}()
	remain = data
	for len(remain) > 0 {
		switch p.state {
		case parserStateFindStartCode:
			p.findStartCode(remain, &remain)

		case parserStateParsePSH:
			p.parsePSH(remain, &remain)
		case parserStateParsePSHStuffing:
			if p.parsePSHStuffing(remain, &remain) {
				return true, remain, nil
			}

		case parserStateParseSYS:
			if _, err := p.parseSYS(remain, &remain); err != nil {
				return false, remain, err
			}
		case parserStateParseSYSStreamInfos:
			if p.parseSYSStreamInfos(remain, &remain) {
				return true, remain, nil
			}

		case parserStateParsePSMPacketLength:
			if _, err := p.parsePSMPacketLength(remain, &remain); err != nil {
				return false, remain, err
			}
		case parserStateParsePSMHeader:
			if ok, err := p.parsePSMHeader(remain, &remain); err != nil {
				return false, remain, err
			} else if ok {
				return true, remain, nil
			}

		case parserStateParsePESPacketLength:
			if _, err := p.parsePESPacketLength(remain, &remain); err != nil {
				return false, remain, err
			}
		case parserStateParseHeaderDataLength:
			if _, err := p.parsePESHeaderDataLength(remain, &remain); err != nil {
				return false, remain, err
			}
		case parserStateParsePESHeader:
			if _, err := p.parsePESHeader(remain, &remain); err != nil {
				return false, remain, err
			}
		case parserStateParsePacketData:
			if p.parsePESPacketData(remain, &remain) {
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

func (p *Parser) Pack() Pack {
	switch p.startCode[3] {
	case 0:
		return nil
	case 0xBA:
		return p.psh
	case 0xBB:
		return p.sys
	case StreamIdProgramStreamMap:
		return p.psm
	default:
		return p.pes
	}
}
