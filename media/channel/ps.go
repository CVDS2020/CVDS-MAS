package channel

import (
	"fmt"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/media/h264"
	"gitee.com/sy_183/cvds-mas/media/ps"
	"gitee.com/sy_183/cvds-mas/storage"
	rtpFramePkg "gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/rtp"
	rtpServer "gitee.com/sy_183/rtp/server"
	"io"
	"math"
	"time"
)

type PsGOP struct {
	psFrames  []*ps.Frame
	rtpFrames []rtpFramePkg.Frame
	start     time.Time
	end       time.Time
	size      int
}

func timeRangeExpandRange(start, end *time.Time, curStart, curEnd time.Time, timeNodeLen int) {
	if start.After(*end) {
		panic(fmt.Errorf("开始时间(%s)在结束时间(%s)之后", *start, *end))
	}
	if curStart.After(curEnd) {
		panic(fmt.Errorf("开始时间(%s)在结束时间(%s)之后", curStart, curEnd))
	}
	if start.IsZero() {
		*start = curStart
		*end = curEnd
	} else {
		du := curStart.Sub(*end)
		if du < 0 {
			guess := time.Duration(0)
			if timeNodeLen > 2 {
				guess = (end.Sub(*start)) / time.Duration(timeNodeLen-2)
			} else if timeNodeLen == 2 {
				guess = curEnd.Sub(curStart)
			}
			offset := du - guess
			*start = start.Add(offset)
		}
		*end = curEnd
	}
}

func (g *PsGOP) Len() int {
	return len(g.rtpFrames)
}

func (g *PsGOP) Append(psFrame *ps.Frame, rtpFrame rtpFramePkg.Frame) {
	g.psFrames = append(g.psFrames, psFrame)
	g.rtpFrames = append(g.rtpFrames, rtpFrame)
	rtpFrame.Range(func(i int, packet rtp.Packet) bool {
		g.size += packet.Payload().Size()
		return true
	})
	icRtpFrame := rtpFrame.(*rtpFramePkg.IncomingFrame)
	timeRangeExpandRange(&g.start, &g.end, icRtpFrame.Start(), icRtpFrame.End(), len(g.rtpFrames))
}

func (g *PsGOP) Start() time.Time {
	return g.start
}

func (g *PsGOP) End() time.Time {
	return g.end
}

func (g *PsGOP) Size() int {
	return g.size
}

func (g *PsGOP) WriteTo(w io.Writer) (n int64, err error) {
	for _, frame := range g.rtpFrames {
		if !frame.Range(func(i int, packet rtp.Packet) bool {
			if err = ioUtils.WriteTo(packet.Payload(), w, &n); err != nil {
				return false
			}
			return true
		}) {
			return
		}
	}
	return n, nil
}

func (g *PsGOP) Clear() {
	for _, psFrame := range g.psFrames {
		psFrame.Release()
	}
	g.psFrames = g.psFrames[:0]
	for _, rtpFrame := range g.rtpFrames {
		rtpFrame.Release()
	}
	g.rtpFrames = g.rtpFrames[:0]
	g.start = time.Time{}
	g.end = time.Time{}
	g.size = 0
}

type streamContext struct {
	mediaType      *media.MediaType
	streamPushers  []*RtpStreamPusher
	outputRtpFrame rtpFramePkg.Frame
	hasData        bool
	disable        bool
}

type psFrameHandler struct {
	frameHandler *FrameHandler
	channel      *Channel

	gop PsGOP

	streamContextMap map[uint8]*streamContext

	parser      *ps.FrameParser
	keepChooser rtpServer.KeepChooser

	h264NaluParser  *h264.AnnexBInPrefixNALUParser
	h264RtpPackager *h264.RtpPackager

	outputRtpFramePool pool.Pool[*rtpFramePkg.DefaultFrame]

	log.LoggerProvider
}

func newPsFrameHandler(frameHandler *FrameHandler) *psFrameHandler {
	return &psFrameHandler{
		frameHandler: frameHandler,
		channel:      frameHandler.channel,

		streamContextMap: make(map[uint8]*streamContext),

		parser:      ps.NewFrameParser(),
		keepChooser: rtpServer.NewDefaultKeepChooser(3, 3, nil),

		outputRtpFramePool: pool.ProvideSyncPool[*rtpFramePkg.DefaultFrame](rtpFramePkg.NewDefaultFrame, pool.WithLimit(math.MaxInt64)),

		LoggerProvider: frameHandler.LoggerProvider,
	}
}

func (h *psFrameHandler) getH264NaluParser() *h264.AnnexBInPrefixNALUParser {
	if h.h264NaluParser == nil {
		h.h264NaluParser = h264.NewAnnexBInPrefixNALUParser(0)
	}
	return h.h264NaluParser
}

func (h *psFrameHandler) getH264RtpPackager() *h264.RtpPackager {
	if h.h264RtpPackager == nil {
		h.h264RtpPackager = h264.NewRtpPackager(0)
	}
	return h.h264RtpPackager
}

func (h *psFrameHandler) getOutputRtpFrame(streamCtx *streamContext) rtpFramePkg.Frame {
	if streamCtx.outputRtpFrame == nil {
		streamCtx.outputRtpFrame = h.outputRtpFramePool.Get()
		if streamCtx.outputRtpFrame == nil {
			return nil
		}
		streamCtx.outputRtpFrame.AddRef()
	}
	return streamCtx.outputRtpFrame
}

func (h *psFrameHandler) parse(rtpFrame rtpFramePkg.Frame, streamInfo *media.RtpStreamInfo) (frame *ps.Frame, keep bool) {
	defer h.parser.Reset()
	keep = true
	var ok bool
	var err error
	h.parser.SetParentStreamInfo(streamInfo)
	rtpFrame.Range(func(i int, packet rtp.Packet) bool {
		return packet.Payload().(*rtp.IncomingPayload).Range(func(chunk []byte) bool {
			for len(chunk) > 0 {
				if ok, err = h.parser.ParseP(chunk, &chunk); ok {
					if err != nil {
						h.Logger().ErrorWith("解析PS帧失败", err)
						if _, is := err.(pool.AllocError); is {
							keep = false
							return false
						}
						if keep = h.keepChooser.OnError(err); !keep {
							h.keepChooser.Reset()
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
		return h.parser.Take(), true
	}
	if h.parser.Complete() {
		return h.parser.Take(), true
	}
	return nil, true
}

func (h *psFrameHandler) handleFrame(rtpFrame rtpFramePkg.Frame, streamInfo *media.RtpStreamInfo) (keep bool) {
	defer rtpFrame.Release()

	psFrame, keep := h.parse(rtpFrame, streamInfo)
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

	psStreamInfos := h.parser.StreamInfos()
	if psStreamInfos == nil {
		// 解析器还获取到流信息，一般为刚开始解析时未解析到关键帧
		return true
	}

	if storageChannel := h.channel.loadEnabledStorageChannel(); storageChannel != nil {
		appendGOP := func() {
			h.gop.Append(psFrame.Use(), rtpFramePkg.UseFrame(rtpFrame))
			if uint(h.gop.Size()) > storageChannel.WriteBufferSize() {
				h.Logger().Warn("PS GOP大小过大，超过存储写缓冲区大小", log.Int("PS GOP大小", h.gop.Size()),
					log.Uint("存储写缓冲区大小", storageChannel.WriteBufferSize()))
				h.gop.Clear()
			}
		}
		if h.gop.Len() == 0 {
			if psFrame.SYS() != nil {
				appendGOP()
			}
		} else {
			if psFrame.SYS() != nil {
				if err := storageChannel.Write(&h.gop, uint(h.gop.Size()), h.gop.Start(), h.gop.End(), storage.StorageTypePS.ID, true); err != nil {
					h.Logger().ErrorWith("写入PS GOP到存储失败", err)
				}
				h.gop.Clear()
			}
			appendGOP()
		}
	} else {
		if h.gop.Len() > 0 {
			h.gop.Clear()
		}
	}

	pushers := h.channel.RtpPushers()
nextPusher:
	for _, pusher := range pushers {
		if !pusher.canPush() {
			continue
		}

		streamPushers := pusher.StreamPushers()
		// 查找推流服务中是否包含PS流通道，如果包含则直接将输入的RTP帧发送
		for _, streamPusher := range streamPushers {
			if streamPusher.StreamInfo().MediaType().ID == media.MediaTypePS.ID {
				pusher.push(rtpFramePkg.UseFrame(rtpFrame), streamPusher.Id())
				continue nextPusher
			}
		}

		// 获取PS流子流信息并与推流通道对应
		for _, streamPusher := range streamPushers {
			pusherStreamInfo := streamPusher.StreamInfo()
			mediaType := pusherStreamInfo.MediaType()
			if psStreamInfo := psStreamInfos.GetStreamInfoByMediaType(mediaType.ID); psStreamInfo != nil {
				psStreamId := psStreamInfo.StreamId()
				streamCtx := h.streamContextMap[psStreamId]
				if streamCtx == nil {
					h.streamContextMap[psStreamId] = &streamContext{
						mediaType:     mediaType,
						streamPushers: []*RtpStreamPusher{streamPusher},
					}
				} else {
					streamCtx.streamPushers = append(streamCtx.streamPushers, streamPusher)
				}
			}
		}
	}

	if len(h.streamContextMap) == 0 {
		return true
	}
	defer func() {
		for _, streamCtx := range h.streamContextMap {
			if streamCtx.outputRtpFrame != nil {
				streamCtx.outputRtpFrame.Release()
			}
		}
		for psStreamId := range h.streamContextMap {
			delete(h.streamContextMap, psStreamId)
		}
	}()

	for _, pes := range psFrame.PES() {
		psStreamId := pes.StreamId()
		psStreamInfo := psStreamInfos.GetStreamInfoById(psStreamId)
		if psStreamInfo == nil {
			continue
		}

		streamCtx := h.streamContextMap[psStreamId]
		if streamCtx != nil && !streamCtx.disable {
			switch streamCtx.mediaType.ID {
			case media.MediaTypeH264.ID:
				h264NaluParser := h.getH264NaluParser()
				h264RtpPackager := h.getH264RtpPackager()
				isPrefix := true
				keep = true
				pes.PackageData().Range(func(chunk []byte) bool {
					if !streamCtx.hasData {
						streamCtx.hasData = true
					}
					ok, err := h264NaluParser.Parse(chunk, isPrefix)
					if isPrefix {
						isPrefix = false
					}
					if ok {
						naluHeader, naluBody := h264NaluParser.NALU()
						outputRtpFrame := h.getOutputRtpFrame(streamCtx)
						if outputRtpFrame == nil {
							h.Logger().ErrorWith("H264 RTP打包失败", pool.NewAllocError("RTP帧"))
							streamCtx.disable = true
							return false
						}
						if err := h264RtpPackager.Package(naluHeader, naluBody, streamCtx.outputRtpFrame.Append); err != nil {
							h.Logger().ErrorWith("H264 RTP打包失败", err)
							streamCtx.disable = true
							return false
						}
					}
					if err != nil {
						h.Logger().ErrorWith("解析H264 NALU失败", err)
						if keep = h.keepChooser.OnError(err); !keep {
							h.keepChooser.Reset()
							return false
						}
					}
					return true
				})
				if !keep {
					return false
				}
			default:
				continue
			}
		}
	}

	for _, streamCtx := range h.streamContextMap {
		if streamCtx.hasData && !streamCtx.disable {
			switch streamCtx.mediaType.ID {
			case media.MediaTypeH264.ID:
				h264NaluParser := h.getH264NaluParser()
				h264RtpPackager := h.getH264RtpPackager()
				if h264NaluParser.Complete() {
					naluHeader, naluBody := h264NaluParser.NALU()
					outputRtpFrame := h.getOutputRtpFrame(streamCtx)
					if outputRtpFrame == nil {
						h.Logger().ErrorWith("H264 RTP打包失败", pool.NewAllocError("RTP帧"))
						streamCtx.disable = true
						continue
					}
					if err := h264RtpPackager.Package(naluHeader, naluBody, streamCtx.outputRtpFrame.Append); err != nil {
						h.Logger().ErrorWith("H264 RTP打包失败", err)
						streamCtx.disable = true
						continue
					}
				}
				if outputRtpFrame := streamCtx.outputRtpFrame; outputRtpFrame != nil {
					outputRtpFrame.SetTimestamp(rtpFrame.Timestamp())
					outputRtpFrame.AddRelation(psFrame.Use())
					outputRtpFrame.AddRelation(rtpFramePkg.UseFrame(rtpFrame))
					for _, streamPusher := range streamCtx.streamPushers {
						streamPusher.Pusher().push(rtpFramePkg.UseFrame(outputRtpFrame), streamPusher.Id())
					}
				}
			}
		}
	}

	return true
}

func (h *psFrameHandler) free() {
	h.parser.Free()
	h.gop.Clear()
}
