package channel

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/utils"
	"gitee.com/sy_183/cvds-mas/media"
	rtpFrame "gitee.com/sy_183/rtp/frame"
	rtpServer "gitee.com/sy_183/rtp/server"
	"sync/atomic"
)

type rtpFrameInfoWrapper struct {
	rtpFrame.Frame
	streamInfo *media.RtpStreamInfo
}

type FrameHandler struct {
	lifecycle.Lifecycle
	once atomic.Bool

	channel *Channel
	player  *RtpPlayer2

	frameChannel chan rtpFrameInfoWrapper

	psFrameParser   *PsRtpFrameParser
	psKeepChooser   rtpServer.KeepChooser
	pesRtpPackagers map[uint32]PesRtpPackager

	outputFramePool pool.Pool[*rtpFrame.DefaultFrame]

	log.LoggerProvider
}

func NewFrameHandler(player *RtpPlayer2) *FrameHandler {
	h := &FrameHandler{
		channel:         player.Channel(),
		player:          player,
		frameChannel:    make(chan rtpFrameInfoWrapper, 10),
		outputFramePool: pool.ProvideSyncPool(rtpFrame.NewDefaultFrame),
		LoggerProvider:  &player.AtomicLogger,
	}
	h.Lifecycle = lifecycle.NewWithInterruptedRun(h.start, h.run)
	return h
}

func (h *FrameHandler) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if !h.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError("")
	}
	return nil
}

func (h *FrameHandler) getPsFrameParser() *PsRtpFrameParser {
	if h.psFrameParser == nil {
		h.psFrameParser = NewPsRtpFrameParser(func(err error) {
			h.Logger().ErrorWith("解析PS数据错误", err)
		})
	}
	return h.psFrameParser
}

func (h *FrameHandler) handlePsFrame(frame rtpFrame.Frame, streamInfo *media.RtpStreamInfo) (keep bool) {
	psFrame, keep := h.getPsFrameParser().Parse(rtpFrameInfoWrapper{Frame: frame, streamInfo: streamInfo})
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

	psStreamInfos := h.psFrameParser.StreamInfos()
	if psStreamInfos == nil {
		// 解析器还获取到流信息，一般为刚开始解析时未解析到关键帧
		return true
	}
	type pusherStream struct {
		streamId   uint64
		streamInfo *media.RtpStreamInfo
	}

	pesOutputRtpFrameMap := make(map[uint8]rtpFrame.Frame)
	for _, pusher := range h.channel.RtpPusher2s() {
		if pusher.canPush() {
			pusherStreamInfoMap := pusher.StreamInfoMap()
			pusherTypeInfoMap := make(map[uint32]uint64)
			for streamId, streamInfo := range pusherStreamInfoMap {
				pusherTypeInfoMap[streamInfo.MediaType().ID] = streamId
			}

			if streamId, exist := pusherTypeInfoMap[media.MediaTypePS.ID]; exist {
				// 推流服务推流类型也为PS流，不需要任何处理，直接分发
				pusher.push(rtpFrame.UseFrame(frame), streamId)
				continue
			}

			// 获取PS流子流对应的RTP流信息和Pusher的流ID，即PS分流映射关系，例如：
			// PS/H264 -> RTP/H264 -> pusher:stream#1
			// PS/G711 -> RTP/PCMA -> pusher:stream#2
			pushMap := make(map[uint8]pusherStream)
			for streamId, streamInfo := range pusherStreamInfoMap {
				mediaType := streamInfo.MediaType()
				// 根据媒体类型获取Pusher流ID对应的PS子流信息
				if psStreamInfo := psStreamInfos.GetStreamInfoByMediaType(mediaType.ID); psStreamInfo != nil {
					if outputFrame := pesOutputRtpFrameMap[psStreamInfo.StreamId()]; outputFrame != nil {
						// 此PS子流已经被打包成RTP帧，直接转发
						pusher.push(rtpFrame.UseFrame(outputFrame), streamId)
					} else {
						pushMap[psStreamInfo.StreamId()] = pusherStream{streamId: streamId, streamInfo: streamInfo}
					}
				}
			}

			if len(pushMap) > 0 {
				for _, pes := range psFrame.PES() {
					if stream := pushMap[pes.StreamId()]; stream.streamInfo != nil {
						mediaType := stream.streamInfo.MediaType()
						pesRtpPackager, has := h.pesRtpPackagers[mediaType.ID]
						if !has {
							pesRtpPackager = FindPesRtpPackagerProvider(mediaType.ID)()
							h.pesRtpPackagers[mediaType.ID] = pesRtpPackager
						}
						if pesRtpPackager == nil {
							continue
						}
						outputFrame := pesOutputRtpFrameMap[pes.StreamId()]
						if outputFrame == nil {
							outputFrame = h.outputFramePool.Get()
							if outputFrame == nil {
								continue
							}
							pesOutputRtpFrameMap[pes.StreamId()] = rtpFrame.UseFrame(outputFrame)
						}
						pesRtpPackager.Package(pes, outputFrame.Append)
						outputFrame.SetTimestamp(frame.Timestamp())
						outputFrame.SetPayloadType(mediaType.PT)
					}
				}
			}

		}
	}
	return false
}

func (h *FrameHandler) handleFrame(frame rtpFrame.Frame, streamInfo *media.RtpStreamInfo) (close bool) {
	defer frame.Release()
	switch streamInfo.MediaType().ID {
	case media.MediaTypePS.ID:
		return h.handlePsFrame(frame, streamInfo)
	case media.MediaTypeH264.ID:
	case media.MediaTypeG711A.ID:
	default:
	}
	return false
}

func (h *FrameHandler) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	defer func() {
		close(h.frameChannel)
		for frame := range h.frameChannel {
			frame.Release()
		}
		if h.psFrameParser != nil {
			h.psFrameParser.Free()
		}
	}()
	for {
		select {
		case frame := <-h.frameChannel:
			if h.handleFrame(frame.Frame, frame.streamInfo) {
				return nil
			}
		case <-interrupter:
			return nil
		}
	}
}

func (h *FrameHandler) Push(frame rtpFrame.Frame, streamInfo *media.RtpStreamInfo) bool {
	defer func() {
		if e := recover(); e != nil {
			frame.Release()
		}
	}()
	if !utils.ChanTryPush(h.frameChannel, rtpFrameInfoWrapper{Frame: frame, streamInfo: streamInfo}) {
		frame.Release()
	}
	return true
}
