package h264

import (
	"gitee.com/sy_183/common/pool"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"gitee.com/sy_183/rtp/rtp"
	"math"
	"sync/atomic"
)

type RtpPackager struct {
	maxRtpPayload int
	packetPool    pool.Pool[*rtp.DefaultPacket]
}

func NewRtpPackager(maxRtpPayload int) *RtpPackager {
	if maxRtpPayload == 0 {
		maxRtpPayload = 1400
	}
	if maxRtpPayload < 1024 {
		maxRtpPayload = 1024
	}
	return &RtpPackager{
		maxRtpPayload: maxRtpPayload,
		packetPool: pool.ProvideSyncPool(rtp.DefaultPacketProvider(func() rtp.Payload {
			return ioUtils.NewOpWriter(make([]byte, 128))
		}), pool.WithLimit(math.MaxInt64)),
	}
}

func (p *RtpPackager) packageUseSingleNALU(naluHeader NALUHeader, naluBody NALUBody, rtpAppender func(packet rtp.Packet) bool) error {
	packet := p.packetPool.Get()
	if packet == nil {
		return pool.NewAllocError("RTP包")
	}
	packet.AddRef()

	payload := packet.Payload().(*ioUtils.OpWriter)
	payload.AppendByte(byte(naluHeader))
	naluBody.Range(func(chunk []byte) bool {
		payload.AppendBytes(chunk)
		return true
	})

	packet.SetMarker(true)
	packet.SetPayload(payload)
	rtpAppender(packet)

	return nil
}

var chunkCount atomic.Int64

func (p *RtpPackager) packageUseFuNALU(naluHeader NALUHeader, naluBody NALUBody, rtpAppender func(packet rtp.Packet) bool) (err error) {
	var packet *rtp.DefaultPacket
	var fuNaluHeader NALUHeader
	var fuHeader FUHeader
	var cache [][]byte
	var cacheSize int

	// 申请新的RTP包，设置NALU头部和FU头部，第一次执行时会在FU头部设置起始标志
	start := true
	initPacket := func() error {
		packet = p.packetPool.Get()
		if packet == nil {
			return pool.NewAllocError("RTP包")
		}
		packet.AddRef()

		fuNaluHeader = NALUHeader(0)
		fuNaluHeader.SetNRI(naluHeader.NRI())
		fuNaluHeader.SetType(NALUTypeFUA)

		fuHeader = FUHeader(0)
		fuHeader.SetStart(start)
		fuHeader.SetType(naluHeader.Type())
		if start {
			start = false
		}
		return nil
	}

	// 将cache中的数据打包到RTP包中，并清空cache
	doPackage := func(end bool) {
		defer func() {
			packet = nil
			cache, cacheSize = cache[:0], 0
		}()
		payload := packet.Payload().(*ioUtils.OpWriter)
		fuHeader.SetEnd(end)
		payload.AppendByte(byte(fuNaluHeader)).AppendByte(byte(fuHeader))
		for _, chunk := range cache {
			payload.AppendBytes(chunk)
		}
		packet.SetMarker(end)
		packet.SetPayload(payload)
		rtpAppender(packet)
	}

	naluBody.Range(func(chunk []byte) bool {
		for len(chunk) > 0 {
			limit := p.maxRtpPayload - cacheSize - 2
			if limit <= 2 {
				doPackage(false)
			}

			if packet == nil {
				if err = initPacket(); err != nil {
					return false
				}
			}

			limit = p.maxRtpPayload - cacheSize - 2
			if limit > len(chunk) {
				limit = len(chunk)
			}
			cache = append(cache, chunk[:limit])
			cacheSize += limit
			chunk = chunk[limit:]
		}

		return true
	})
	doPackage(true)

	return nil
}

func (p *RtpPackager) Package(naluHeader NALUHeader, naluBody NALUBody, rtpAppender func(packet rtp.Packet) bool) error {
	if naluBody.Size()+1 < p.maxRtpPayload {
		// 使用单一NALU包
		return p.packageUseSingleNALU(naluHeader, naluBody, rtpAppender)
	}
	// NALU大小超过了最大RTP负载大小，使用FU组合NALU包
	return p.packageUseFuNALU(naluHeader, naluBody, rtpAppender)
}
