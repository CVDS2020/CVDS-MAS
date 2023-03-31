package ps

import (
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/pool"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"gitee.com/sy_183/rtp/rtp"
	"math"
)

type RtpPackager struct {
	maxRtpPayload  int
	packetPool     pool.Pool[*rtp.DefaultPacket]
	pesIsStreamEnd map[uint8]int
}

func NewRtpPackager(maxRtpPayload int) *RtpPackager {
	if maxRtpPayload == 0 {
		maxRtpPayload = 1400
	}
	if maxRtpPayload < 1024 {
		maxRtpPayload = 1024
	}
	return &RtpPackager{
		maxRtpPayload:  maxRtpPayload,
		packetPool:     pool.ProvideSyncPool(rtp.DefaultPacketProvider(rtp.ProvideIncomingPayload), pool.WithLimit(math.MaxInt64)),
		pesIsStreamEnd: make(map[uint8]int),
	}
}

func (p *RtpPackager) packageMetaData(metaData []byte, rtpAppender func(packet rtp.Packet) bool) error {
	for len(metaData) > 0 {
		packet := p.packetPool.Get()
		if packet == nil {
			return pool.NewAllocError("RTP包")
		}
		packet.AddRef()

		limit := len(metaData)
		if limit > p.maxRtpPayload {
			limit = p.maxRtpPayload
		}
		payload := packet.Payload().(*rtp.IncomingPayload)
		payload.AddContent(metaData[:limit])
		rtpAppender(packet)
		metaData = metaData[limit:]
	}
	return nil
}

func (p *RtpPackager) packagePESPacketData(pes *PES, isStreamEnd bool, rtpAppender func(packet rtp.Packet) bool) (err error) {
	var packet *rtp.DefaultPacket
	var cache [][]byte
	var cacheSize int

	// 将cache中的数据打包到RTP包中，并清空cache
	doPackage := func(end bool) {
		defer func() {
			packet = nil
			cache, cacheSize = cache[:0], 0
		}()
		payload := packet.Payload().(*rtp.IncomingPayload)
		for _, chunk := range cache {
			payload.AddContent(chunk)
		}
		packet.SetMarker(end)
		rtpAppender(packet)
	}

	pes.PackageData().Range(func(chunk []byte) bool {
		for len(chunk) > 0 {
			limit := p.maxRtpPayload - cacheSize
			if limit == 0 {
				doPackage(false)
			}

			if packet == nil {
				packet = p.packetPool.Get()
				if packet == nil {
					err = pool.NewAllocError("RTP包")
					return false
				}
				packet.AddRef()
			}

			limit = p.maxRtpPayload - cacheSize
			if limit > len(chunk) {
				limit = len(chunk)
			}
			cache = append(cache, chunk[:limit])
			cacheSize += limit
			chunk = chunk[limit:]
		}
		return true
	})
	if packet != nil {
		doPackage(isStreamEnd)
	}
	return nil
}

func (p *RtpPackager) Package(frame *Frame, rtpAppender func(packet rtp.Packet) bool) (err error) {
	if frame.PSH() == nil {
		return errors.New("PS帧缺少PSH头部")
	}

	metaSize := frame.PSH().Size()
	if frame.SYS() != nil {
		metaSize += frame.SYS().Size()
	}
	if frame.PSM() != nil {
		metaSize += frame.PSM().Size()
	}

	for i, pes := range frame.PES() {
		p.pesIsStreamEnd[pes.StreamId()] = i
	}
	defer func() {
		for id := range p.pesIsStreamEnd {
			delete(p.pesIsStreamEnd, id)
		}
	}()

	for i, pes := range frame.PES() {
		// 将PES的头部写入RTP包中，如果是第一个PES包，先将PSH、SYS、PSM写入RTP包
		metaSize += pes.Header().Size()
		metaWriter := ioUtils.Writer{Buf: make([]byte, metaSize)}
		if i == 0 {
			frame.PSH().WriteTo(&metaWriter)
			if frame.SYS() != nil {
				frame.SYS().WriteTo(&metaWriter)
			}
			if frame.PSM() != nil {
				frame.PSM().WriteTo(&metaWriter)
			}
		}
		pes.Header().WriteTo(&metaWriter)
		if err = p.packageMetaData(metaWriter.Bytes(), rtpAppender); err != nil {
			return err
		}
		metaSize = 0

		// 将PES数据写入RTP包
		if err = p.packagePESPacketData(pes, p.pesIsStreamEnd[pes.StreamId()] == i, rtpAppender); err != nil {
			return err
		}
	}

	return nil
}
