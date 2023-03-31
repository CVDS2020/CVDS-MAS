package channel

import (
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/utils"
	"gitee.com/sy_183/cvds-mas/media"
	rtpFrame "gitee.com/sy_183/rtp/frame"
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
	player  *RtpPlayer

	frameChannel   chan rtpFrameInfoWrapper
	psFrameHandler *psFrameHandler

	log.LoggerProvider
}

func NewFrameHandler(player *RtpPlayer) *FrameHandler {
	h := &FrameHandler{
		channel:        player.Channel(),
		player:         player,
		frameChannel:   make(chan rtpFrameInfoWrapper, 10),
		LoggerProvider: &player.AtomicLogger,
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

func (h *FrameHandler) getPsFrameHandler() *psFrameHandler {
	if h.psFrameHandler == nil {
		h.psFrameHandler = newPsFrameHandler(h)
	}
	return h.psFrameHandler
}

func (h *FrameHandler) handleFrame(frame rtpFrame.Frame, streamInfo *media.RtpStreamInfo) (keep bool) {
	switch streamInfo.MediaType().ID {
	case media.MediaTypePS.ID:
		return h.getPsFrameHandler().handleFrame(frame, streamInfo)
	default:
		frame.Release()
	}
	return true
}

func (h *FrameHandler) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	defer func() {
		close(h.frameChannel)
		for frame := range h.frameChannel {
			frame.Release()
		}
		if h.psFrameHandler != nil {
			h.psFrameHandler.free()
		}
	}()
	for {
		select {
		case frame := <-h.frameChannel:
			if !h.handleFrame(frame.Frame, frame.streamInfo) {
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
