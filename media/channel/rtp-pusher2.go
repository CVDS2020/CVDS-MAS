package channel

import (
	"bytes"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/utils"
	"gitee.com/sy_183/cvds-mas/media"
	rtpFrame "gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/rtp"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	DefaultPusherMaxCached = 4
	MaxPusherMaxCached     = 50
)

type (
	RtpPusher2ClosedCallback func(pusher *RtpPusher2, channel *Channel)
)

type rtpFramePusherWrapper struct {
	rtpFrame.Frame
	streamId uint64
}

type rtpStreamPusher struct {
	id      uint64
	pusher  *RtpPusher2
	removed bool

	transport  string
	localIp    net.IP
	localPort  int
	remoteIp   net.IP
	remotePort int
	ssrc       uint32

	tcpConn *net.TCPConn
	udpConn *net.UDPConn
	buffer  bytes.Buffer

	streamInfo atomic.Pointer[media.RtpStreamInfo]

	log.LoggerProvider
}

func newRtpStreamPusher(id uint64, pusher *RtpPusher2, transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (*rtpStreamPusher, error) {
	if ipv4 := remoteIp.To4(); ipv4 != nil {
		remoteIp = ipv4
	}

	switch strings.ToLower(transport) {
	case "tcp":
		transport = "tcp"
	default:
		transport = "udp"
	}

	var tcpConn *net.TCPConn
	var udpConn *net.UDPConn
	var localIp net.IP
	var localPort int
	switch transport {
	case "tcp":
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   remoteIp,
			Port: remotePort,
		})
		if err != nil {
			return nil, err
		}
		tcpConn = conn
		localAddr := conn.LocalAddr().(*net.TCPAddr)
		if ipv4 := localAddr.IP.To4(); ipv4 != nil {
			localIp = ipv4
		} else {
			localIp = localAddr.IP
		}
		localPort = localAddr.Port
	case "udp":
		conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
			IP:   remoteIp,
			Port: remotePort,
		})
		if err != nil {
			return nil, err
		}
		udpConn = conn
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		if ipv4 := localAddr.IP.To4(); ipv4 != nil {
			localIp = ipv4
		} else {
			localIp = localAddr.IP
		}
		localPort = localAddr.Port
	}

	p := &rtpStreamPusher{
		id:     id,
		pusher: pusher,

		transport:  transport,
		localIp:    localIp,
		localPort:  localPort,
		remoteIp:   remoteIp,
		remotePort: remotePort,

		tcpConn: tcpConn,
		udpConn: udpConn,

		LoggerProvider: &pusher.AtomicLogger,
	}

	p.streamInfo.Store(streamInfo)
	if ssrc < 0 {
		p.ssrc = rand.Uint32()
	} else {
		p.ssrc = uint32(ssrc)
	}
	return p, nil
}

func (p *rtpStreamPusher) send(frame rtpFrame.Frame) (err error) {
	buffer := &p.buffer
	defer func() {
		buffer.Reset()
		frame.Release()
	}()
	if p.udpConn != nil {
		frame.Range(func(i int, packet rtp.Packet) bool {
			packet.WriteTo(&p.buffer)
			if _, err = p.udpConn.Write(buffer.Bytes()); err != nil {
				p.Logger().ErrorWith("发送基于UDP传输的RTP数据失败", err)
				return false
			}
			return true
		})
	} else {
		rtpFrame.NewFullFrameWriter(frame).WriteTo(buffer)
		if _, err = p.tcpConn.Write(buffer.Bytes()); err != nil {
			p.Logger().ErrorWith("发送基于TCP传输的RTP数据失败", err)
			return err
		}
	}
	return nil
}

func (p *rtpStreamPusher) close() {
	if p.tcpConn != nil {
		p.tcpConn.Close()
	}
	if p.udpConn != nil {
		p.udpConn.Close()
	}
}

type RtpPusher2 struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle
	once   atomic.Bool

	id      uint64
	channel *Channel

	streamPusherId    atomic.Uint64
	streamPusherCount atomic.Int64
	streamPushers     container.SyncMap[uint64, *rtpStreamPusher]
	streamPusherLock  sync.Mutex

	frameChannel chan rtpFramePusherWrapper
	canPushFlag  atomic.Bool
	closeFlag    atomic.Bool
	pushSignal   chan struct{}
	pauseSignal  chan struct{}

	log.AtomicLogger
}

func NewRtpPusher2(id uint64, channel *Channel, maxCached int) *RtpPusher2 {
	if maxCached <= 0 {
		maxCached = DefaultPusherMaxCached
	} else if maxCached > MaxPusherMaxCached {
		maxCached = MaxPusherMaxCached
	}
	p := &RtpPusher2{
		id:      id,
		channel: channel,

		frameChannel: make(chan rtpFramePusherWrapper, maxCached),
		pushSignal:   make(chan struct{}, 1),
		pauseSignal:  make(chan struct{}, 1),
	}

	p.pauseSignal <- struct{}{}

	p.SetLogger(channel.Logger().Named(p.DisplayName()))
	p.runner = lifecycle.NewWithInterruptedRun(nil, p.run)
	p.Lifecycle = p.runner
	return p
}

func (p *RtpPusher2) DisplayName() string {
	return p.channel.RtpPusherDisplayName(p.id)
}

func (p *RtpPusher2) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if !p.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError(p.DisplayName())
	}
	return nil
}

func (p *RtpPusher2) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
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

func (p *RtpPusher2) AddStream(transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (streamId uint64, err error) {
	return lock.RLockGetDouble(p.runner, func() (streamId uint64, err error) {
		if !p.runner.Running() {
			return 0, p.Logger().ErrorWith("添加RTP推流失败", lifecycle.NewStateNotRunningError(p.channel.DisplayName()))
		}
		streamId = p.streamPusherId.Add(1)
		streamPusher, err := newRtpStreamPusher(streamId, p, transport, remoteIp, remotePort, ssrc, streamInfo)
		if err != nil {
			return 0, err
		}
		lock.LockDo(&p.streamPusherLock, func() {
			p.streamPushers.Store(streamId, streamPusher)
			p.streamPusherCount.Add(1)
		})
		return streamId, nil
	})
}

func (p *RtpPusher2) RemoveStream(streamId uint64) error {
	streamPusher, closePusher := lock.LockGetDouble(&p.streamPusherLock, func() (*rtpStreamPusher, bool) {
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

func (p *RtpPusher2) StreamInfoMap() map[uint64]*media.RtpStreamInfo {
	infoMap := make(map[uint64]*media.RtpStreamInfo)
	for _, pusher := range p.streamPushers.Values() {
		infoMap[pusher.id] = pusher.streamInfo.Load()
	}
	return infoMap
}

func (p *RtpPusher2) canPush() bool {
	return p.canPushFlag.Load() && p.closeFlag.Load()
}

func (p *RtpPusher2) push(frame rtpFrame.Frame, streamId uint64) {
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

func (p *RtpPusher2) StartPush() {
	if !utils.ChanTryPush(p.pushSignal, struct{}{}) {
		p.Logger().Warn("开始推流信号繁忙")
	} else {
		p.canPushFlag.Store(true)
	}
}

func (p *RtpPusher2) Pause() {
	if !utils.ChanTryPush(p.pauseSignal, struct{}{}) {
		p.Logger().Warn("暂停信号繁忙")
	} else {
		p.canPushFlag.Store(false)
	}
}
