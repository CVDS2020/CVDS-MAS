package channel

import (
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/utils"
	"gitee.com/sy_183/cvds-mas/media"
	rtpFrame "gitee.com/sy_183/rtp/frame"
	"net"
	"sync"
	"sync/atomic"
)

const (
	DefaultPusherMaxCached = 4
	MaxPusherMaxCached     = 50
)

type (
	RtpPusherClosedCallback func(pusher *RtpPusher, channel *Channel)
)

type rtpFramePusherWrapper struct {
	rtpFrame.Frame
	streamId uint64
}

type RtpPusher struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle
	once   atomic.Bool

	id      uint64
	channel *Channel

	streamPusherId    atomic.Uint64
	streamPusherCount atomic.Int64
	streamPushers     container.SyncMap[uint64, *RtpStreamPusher]
	streamPusherLock  sync.Mutex

	maxCached    int
	frameChannel chan rtpFramePusherWrapper

	canPushFlag atomic.Bool
	closeFlag   atomic.Bool
	pushSignal  chan struct{}
	pauseSignal chan struct{}

	log.AtomicLogger
}

func newRtpPusher(id uint64, channel *Channel, maxCached int) *RtpPusher {
	if maxCached <= 0 {
		maxCached = DefaultPusherMaxCached
	} else if maxCached > MaxPusherMaxCached {
		maxCached = MaxPusherMaxCached
	}
	p := &RtpPusher{
		id:      id,
		channel: channel,

		maxCached:    maxCached,
		frameChannel: make(chan rtpFramePusherWrapper, maxCached),

		pushSignal:  make(chan struct{}, 1),
		pauseSignal: make(chan struct{}, 1),
	}

	p.pauseSignal <- struct{}{}

	p.SetLogger(channel.Logger().Named(p.DisplayName()))
	p.runner = lifecycle.NewWithInterruptedRun(nil, p.run)
	p.Lifecycle = p.runner
	return p
}

func (p *RtpPusher) Id() uint64 {
	return p.id
}

func (p *RtpPusher) DisplayName() string {
	return p.channel.RtpPusherDisplayName(p.id)
}

func (p *RtpPusher) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if !p.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError(p.DisplayName())
	}
	return nil
}

func (p *RtpPusher) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	defer func() {
		p.closeFlag.Store(true)
		close(p.frameChannel)
		for frame := range p.frameChannel {
			frame.Release()
		}
		if streamPushers := p.streamPushers.Values(); len(streamPushers) > 0 {
			for _, streamPusher := range streamPushers {
				p.RemoveStream(streamPusher.id)
			}
		}
	}()

	for {
		select {
		case <-p.pauseSignal:
		pause:
			for {
				select {
				case <-interrupter:
					return nil
				case <-p.pauseSignal:
				case <-p.pushSignal:
					break pause
				}
			}
		case <-p.pushSignal:
		case frame := <-p.frameChannel:
			if streamPusher, _ := p.streamPushers.Load(frame.streamId); streamPusher != nil {
				if err := streamPusher.send(frame.Frame); err != nil {
					p.Logger().ErrorWith("发送RTP数据失败", err)
					p.RemoveStream(frame.streamId)
				}
			} else {
				frame.Release()
			}
		case <-interrupter:
			return nil
		}
	}
}

func (p *RtpPusher) StreamPushers() []*RtpStreamPusher {
	return p.streamPushers.Values()
}

func (p *RtpPusher) AddStream(transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (streamPusher *RtpStreamPusher, err error) {
	return lock.RLockGetDouble(p.runner, func() (streamPusher *RtpStreamPusher, err error) {
		if !p.runner.Running() {
			return nil, p.Logger().ErrorWith("实时音视频RTP推流通道添加失败", lifecycle.NewStateNotRunningError(p.DisplayName()))
		}
		streamId := p.streamPusherId.Add(1)
		streamPusher, err = newRtpStreamPusher(streamId, p, nil, transport, remoteIp, remotePort, ssrc, streamInfo)
		if err != nil {
			return nil, err
		}
		lock.LockDo(&p.streamPusherLock, func() {
			p.streamPushers.Store(streamId, streamPusher)
			p.streamPusherCount.Add(1)
		})
		return streamPusher, nil
	})
}

func (p *RtpPusher) RemoveStream(streamId uint64) error {
	streamPusher, closePusher := lock.LockGetDouble(&p.streamPusherLock, func() (*RtpStreamPusher, bool) {
		streamPlayer, _ := p.streamPushers.LoadAndDelete(streamId)
		if streamPlayer != nil {
			return streamPlayer, p.streamPusherCount.Add(-1) == 0
		}
		return nil, false
	})
	if streamPusher != nil {
		streamPusher.close()
	}
	if closePusher {
		p.Close(nil)
	}
	return nil
}

func (p *RtpPusher) canPush() bool {
	return p.canPushFlag.Load() && !p.closeFlag.Load()
}

func (p *RtpPusher) push(frame rtpFrame.Frame, streamId uint64) {
	defer func() {
		if e := recover(); e != nil {
			frame.Release()
		}
	}()
	if !p.canPush() {
		frame.Release()
		return
	}
	if !utils.ChanTryPush(p.frameChannel, rtpFramePusherWrapper{Frame: frame, streamId: streamId}) {
		frame.Release()
	}
}

func (p *RtpPusher) StartPush() {
	if !utils.ChanTryPush(p.pushSignal, struct{}{}) {
		p.Logger().Warn("开始推流信号繁忙")
	} else {
		p.canPushFlag.Store(true)
	}
}

func (p *RtpPusher) Pause() {
	if !utils.ChanTryPush(p.pauseSignal, struct{}{}) {
		p.Logger().Warn("暂停信号繁忙")
	} else {
		p.canPushFlag.Store(false)
	}
}

func (p *RtpPusher) Info() map[string]any {
	infos := make([]map[string]any, 0)
	p.streamPushers.Range(func(id uint64, streamPusher *RtpStreamPusher) bool {
		infos = append(infos, streamPusher.Info())
		return true
	})
	return map[string]any{
		"id":            p.id,
		"maxCached":     p.maxCached,
		"streamPushers": infos,
	}
}
