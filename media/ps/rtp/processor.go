package rtp

import (
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/media/ps"
	rtpFramePkg "gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/rtp"
	rtpServer "gitee.com/sy_183/rtp/server"
)

type Handler interface {
	Handle(rtpFrame rtpFramePkg.Frame)
}

type Handlers []Handler

func (hs Handlers) Handle(rtpFrame rtpFramePkg.Frame) {
	for _, handler := range hs {
		handler.Handle(rtpFramePkg.UseFrame(rtpFrame))
	}
}

type FrameProcessor struct {
	parser      *ps.FrameParser
	packagers   map[uint32]Packager
	keepChooser rtpServer.KeepChooser

	streams          []uint8
	streamUsed       [256]bool
	handler          [256]Handlers
	outputFrameCache [256]rtpFramePkg.Frame

	outputFramePool pool.Pool[*rtpFramePkg.DefaultFrame]
}

func (p *FrameProcessor) parse(rtpFrame rtpFramePkg.Frame, streamInfo *media.RtpStreamInfo) (psFrame *ps.Frame, keep bool) {
	defer p.parser.Reset()
	var ok bool
	var err error
	p.parser.SetParentStreamInfo(streamInfo)
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

func (p *FrameProcessor) getOrCreateOutputFrame(streamId uint8) rtpFramePkg.Frame {
	outputFrame := p.outputFrameCache[streamId]
	if outputFrame != nil {
		return outputFrame
	}
	outputFrame = p.outputFramePool.Get()
	if outputFrame == nil {
		return nil
	}
	p.outputFrameCache[streamId] = rtpFramePkg.UseFrame(outputFrame)
	return outputFrame
}

func (p *FrameProcessor) getOrCreatePackager(mediaType uint32) Packager {
	packager, has := p.packagers[mediaType]
	if has {
		return packager
	}
	packager = FindPackagerProvider(mediaType)()
	p.packagers[mediaType] = packager
	return packager
}

func (p *FrameProcessor) Handle(rtpFrame rtpFramePkg.Frame, streamInfo *media.RtpStreamInfo) (keep bool) {
	psFrame, keep := p.parse(rtpFrame, streamInfo)
	if !keep {
		return false
	} else if psFrame == nil {
		return true
	}
	defer psFrame.Release()

	if len(psFrame.PES()) == 0 {
		// PS帧中不包含PES数据
		return true
	}

	psStreamInfos := p.parser.StreamInfos()
	if psStreamInfos == nil {
		// 解析器还获取到流信息，一般为刚开始解析时未解析到关键帧
		return true
	}

	for _, psStreamInfo := range psStreamInfos.StreamInfos() {
		streamId := psStreamInfo.StreamId()
		handler := p.handler[streamId]
		if handler != nil {
			p.streams = append(p.streams, streamId)
			p.streamUsed[streamId] = true
		}
	}

	for _, pes := range psFrame.PES() {
		streamId := pes.StreamId()
		psStreamInfo := psStreamInfos.GetStreamInfoById(streamId)
		if psStreamInfo == nil {
			continue
		}
		if p.streamUsed[streamId] {
			packager := p.getOrCreatePackager(psStreamInfo.MediaType().ID)
			if packager == nil {
				p.streamUsed[streamId] = false
				continue
			}
			outputFrame := p.getOrCreateOutputFrame(streamId)
			if outputFrame == nil {
				p.streamUsed[streamId] = false
				continue
			}
			if err := packager.Package(pes, outputFrame.Append); err != nil {
				p.streamUsed[streamId] = false
				p.outputFrameCache[streamId].Release()
				p.outputFrameCache[streamId] = nil
			}
		}
	}

	for _, streamId := range p.streams {
		if p.streamUsed[streamId] {
			psStreamInfo := psStreamInfos.GetStreamInfoById(streamId)
			if outputFrame := p.outputFrameCache[streamId]; outputFrame != nil {
				packager := p.getOrCreatePackager(psStreamInfo.MediaType().ID)
				if err := packager.Complete(outputFrame.Append); err != nil {
					outputFrame.Release()
					p.outputFrameCache[streamId] = nil
					p.streamUsed[streamId] = false
					continue
				}
				outputFrame.SetTimestamp(rtpFrame.Timestamp())
				outputFrame.SetPayloadType(psStreamInfo.MediaType().PT)
				p.handler[streamId].Handle(outputFrame)
				outputFrame.Release()
				p.outputFrameCache[streamId] = nil
				//p.streamUsed[streamId]
			}
		}
	}

	//for _, streamId := range p.streams {
	//	if p.outputFrameCache[streamId]
	//}

	return true
}
