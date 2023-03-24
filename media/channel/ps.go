package channel

import (
	"errors"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/common/pool"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/media/h264"
	"gitee.com/sy_183/cvds-mas/media/ps"
	"gitee.com/sy_183/rtp/rtp"
	rtpServer "gitee.com/sy_183/rtp/server"
	"math"
)

type PsRtpFrameParser struct {
	parser      *ps.FrameParser
	keepChooser rtpServer.KeepChooser
}

func NewPsRtpFrameParser(onError func(err error)) *PsRtpFrameParser {
	p := &PsRtpFrameParser{
		parser: ps.NewFrameParser(),
		keepChooser: rtpServer.NewDefaultKeepChooser(1, 1, func(err error) bool {
			onError(err)
			return true
		}),
	}
	return p
}

func (p *PsRtpFrameParser) StreamInfos() *ps.StreamInfos {
	return p.parser.StreamInfos()
}

func (p *PsRtpFrameParser) Parse(rtpFrame rtpFrameInfoWrapper) (frame *ps.Frame, keep bool) {
	defer p.parser.Reset()
	var ok bool
	var err error
	p.parser.SetParentStreamInfo(rtpFrame.streamInfo)
	rtpFrame.Range(func(i int, packet rtp.Packet) bool {
		return packet.Payload().(*rtp.IncomingPayload).Range(func(chunk []byte) bool {
			for len(chunk) > 0 {
				if ok, err = p.parser.ParseP(chunk, &chunk); ok {
					if err != nil {
						if keep = p.keepChooser.OnError(err); !keep {
							p.keepChooser.Reset()
							return false
						}
					}
					if ok {
						return false
					}
				}
			}
			return true
		})
	})
	if !keep {
		return nil, false
	}
	if ok {
		return p.parser.Take(), true
	}
	if p.parser.Complete() {
		return p.parser.Take(), true
	}
	return nil, true
}

func (p *PsRtpFrameParser) Free() {
	p.parser.Free()
}

type PesRtpPackager interface {
	Package(pes *ps.PES, rtpAppender func(packet rtp.Packet) bool) error

	Complete(rtpAppender func(packet rtp.Packet) bool) error
}

type PesRtpPackagerProvider func(options ...option.Option[PesRtpPackager, any]) PesRtpPackager

var pesRtpPackagerMap container.SyncMap[uint32, PesRtpPackagerProvider]

func RegisterPesRtpPackagerProvider(mediaType uint32, provider PesRtpPackagerProvider) {
	pesRtpPackagerMap.Store(mediaType, provider)
}

func FindPesRtpPackagerProvider(mediaType uint32) PesRtpPackagerProvider {
	p, _ := pesRtpPackagerMap.Load(mediaType)
	return p
}

func init() {
	RegisterPesRtpPackagerProvider(media.MediaTypeH264.ID, ProvideH264PesRtpPackager)
}

type H264PesRtpPackager struct {
	packetPool     pool.Pool[*rtp.DefaultPacket]
	maxRtpPayload  int
	h264NALUParser *h264.AnnexBInPrefixNALUParser

	naluHeader   h264.NALUHeader
	naluBody     [][]byte
	naluBodySize int
}

func WithMaxRtpPayload(maxRtpPayload int) option.Option[PesRtpPackager, any] {
	type maxRtpPayloadSetter interface {
		setMaxRtpPayload(maxRtpPayload int)
	}
	return option.Custom[PesRtpPackager](func(packager PesRtpPackager) {
		if setter, is := packager.(maxRtpPayloadSetter); is {
			setter.setMaxRtpPayload(maxRtpPayload)
		}
	})
}

func NewH264PesRtpPackager(options ...option.Option[PesRtpPackager, any]) *H264PesRtpPackager {
	p := &H264PesRtpPackager{
		packetPool:     pool.ProvideSyncPool(rtp.NewDefaultPacket, pool.WithLimit(math.MaxInt64)),
		h264NALUParser: h264.NewAnnexBInPrefixNALUParser(0),
	}
	for _, opt := range options {
		opt.Apply(p)
	}
	if p.maxRtpPayload == 0 {
		p.maxRtpPayload = 1400
	}
	if p.maxRtpPayload < 1024 {
		p.maxRtpPayload = 1024
	}
	return p
}

func ProvideH264PesRtpPackager(options ...option.Option[PesRtpPackager, any]) PesRtpPackager {
	return NewH264PesRtpPackager(options...)
}

func (p *H264PesRtpPackager) setMaxRtpPayload(maxRtpPayload int) {
	p.maxRtpPayload = maxRtpPayload
}

func (p *H264PesRtpPackager) doPackage(rtpAppender func(packet rtp.Packet) bool) error {
	if p.naluBodySize+1 <= p.maxRtpPayload {
		// 使用单一NALU包
		packet := p.packetPool.Get()
		if packet == nil {
			return errors.New("申请RTP包失败")
		}
		packet.AddRef()
		payload := &ioUtils.OpWriter{OpBuf: make([]byte, 0, 2+len(p.naluBody)*ioUtils.BytesOpSize)}
		payload.AppendByte(byte(p.naluHeader))
		for _, body := range p.naluBody {
			payload.AppendBytes(body)
		}
		packet.SetMarker(true)
		packet.SetPayload(payload)
		rtpAppender(packet)
		packet.Release()
		return nil
	}
	// 使用FU NALU包
	remain := p.naluBodySize
	start, end := true, false
	naluBodyLinear := NewChunksLinear(p.naluBody, p.naluBodySize)
	for remain > 0 {
		packet := p.packetPool.Get()
		if packet == nil {
			return errors.New("申请RTP包失败")
		}
		packet.AddRef()

		naluBodySize := remain
		if naluBodySize+2 > p.maxRtpPayload {
			naluBodySize = p.maxRtpPayload - 2
		} else {
			end = true
		}
		payload := &ioUtils.OpWriter{}

		naluHeader := h264.NALUHeader(0)
		naluHeader.SetNRI(p.naluHeader.NRI())
		naluHeader.SetType(h264.NALUTypeFUA)

		fuHeader := h264.FUHeader(0)
		fuHeader.SetStart(start)
		fuHeader.SetEnd(end)
		fuHeader.SetType(p.naluHeader.Type())

		if start {
			start = false
		}
		payload.AppendByte(byte(naluHeader)).AppendByte(byte(fuHeader))

		var naluBody ChunksLinear
		naluBodyLinear.CutTo(-1, naluBodySize, &naluBody)
		naluBodyLinear.CutTo(naluBodySize, -1, naluBodyLinear)
		naluBody.Range(func(chunk []byte) bool {
			payload.AppendBytes(chunk)
			return true
		})

		if end {
			packet.SetMarker(true)
		}
		packet.SetPayload(payload)
		rtpAppender(packet)
		packet.Release()
		remain -= naluBodySize
	}
	return nil
}

func (p *H264PesRtpPackager) Package(pes *ps.PES, rtpAppender func(packet rtp.Packet) bool) error {
	for i, chunk := range pes.PackageData() {
		ok, err := p.h264NALUParser.Parse(chunk, i == 0)
		if ok {
			p.naluHeader, p.naluBody, p.naluBodySize = p.h264NALUParser.NALU()
			if err := p.doPackage(rtpAppender); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *H264PesRtpPackager) Complete(rtpAppender func(packet rtp.Packet) bool) error {
	if p.h264NALUParser.Complete() {
		p.naluHeader, p.naluBody, p.naluBodySize = p.h264NALUParser.NALU()
		if err := p.doPackage(rtpAppender); err != nil {
			return err
		}
	}
	return nil
}
