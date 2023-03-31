package channel

import (
	"bytes"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/utils"
	rtpFrame "gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/rtp"
	"math/rand"
	"net"
	"strings"
	"sync/atomic"
)

type (
	RtpPusherClosedCallback func(pusher *RtpPusher, channel *Channel)
)

type RtpPusher struct {
	lifecycle.Lifecycle
	runner  *lifecycle.DefaultLifecycle
	started atomic.Bool

	id      uint64
	channel *Channel

	transport  string
	localIp    net.IP
	localPort  int
	remoteIp   net.IP
	remotePort int
	ssrc       uint32

	tcpConn *net.TCPConn
	udpConn *net.UDPConn

	frameChannel chan rtpFrame.Frame
	pushSignal   chan struct{}

	log.AtomicLogger
}

func NewRtpPusher(id uint64, channel *Channel, remoteIp net.IP, remotePort int, transport string, ssrc int64) (*RtpPusher, error) {
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

	p := &RtpPusher{
		id:      id,
		channel: channel,

		transport:  transport,
		localIp:    localIp,
		localPort:  localPort,
		remoteIp:   remoteIp,
		remotePort: remotePort,

		tcpConn: tcpConn,
		udpConn: udpConn,

		frameChannel: make(chan rtpFrame.Frame, 10),
		pushSignal:   make(chan struct{}, 1),
	}

	if ssrc < 0 {
		p.ssrc = rand.Uint32()
	} else {
		p.ssrc = uint32(ssrc)
	}

	p.SetLogger(channel.Logger().Named(p.DisplayName()))
	p.runner = lifecycle.NewWithInterruptedRun(nil, p.run)
	p.Lifecycle = p.runner
	return p, nil
}

func (p *RtpPusher) DisplayName() string {
	return p.channel.RtpPusherDisplayName(p.id)
}

func (p *RtpPusher) ID() uint64 {
	return p.id
}

func (p *RtpPusher) LocalIp() net.IP {
	return p.localIp
}

func (p *RtpPusher) LocalPort() int {
	return p.localPort
}

func (p *RtpPusher) RemoteIp() net.IP {
	return p.remoteIp
}

func (p *RtpPusher) RemotePort() int {
	return p.remotePort
}

func (p *RtpPusher) SSRC() uint32 {
	return p.ssrc
}

func (p *RtpPusher) Info() map[string]any {
	return map[string]any{
		"pusherId":   p.ID(),
		"localIp":    p.LocalIp(),
		"localPort":  p.LocalPort(),
		"remoteIp":   p.RemoteIp(),
		"remotePort": p.RemotePort(),
		"ssrc":       p.SSRC(),
	}
}

func (p *RtpPusher) send(frame rtpFrame.Frame, buffer *bytes.Buffer) (err error) {
	defer func() {
		buffer.Reset()
		frame.Release()
	}()
	if p.udpConn != nil {
		frame.Range(func(i int, packet rtp.Packet) bool {
			packet.WriteTo(buffer)
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

func (p *RtpPusher) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if p.started.Load() {
		return lifecycle.NewStateClosedError(p.DisplayName())
	}
	for {
		select {
		case <-p.pushSignal:
			p.started.Store(true)
			return nil
		case <-interrupter:
			err := lifecycle.NewInterruptedError(p.DisplayName(), "启动")
			p.Logger().Warn(err.Error())
			return err
		}
	}
}

func (p *RtpPusher) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	defer func() {
		close(p.frameChannel)
		for frame := range p.frameChannel {
			frame.Release()
		}
		if p.tcpConn != nil {
			p.tcpConn.Close()
		}
		if p.udpConn != nil {
			p.udpConn.Close()
		}
	}()
	var writeBuffer = bytes.Buffer{}
	for {
		select {
		case frame := <-p.frameChannel:
			if err := p.send(frame, &writeBuffer); err != nil {
				return err
			}
		case <-interrupter:
			return nil
		}
	}
}

func (p *RtpPusher) StartPush() {
	if !utils.ChanTryPush(p.pushSignal, struct{}{}) {
		p.Logger().Warn("开始推流信号繁忙")
	}
}

func (p *RtpPusher) Started() bool {
	return p.started.Load()
}

func (p *RtpPusher) Push(frame rtpFrame.Frame) {
	defer func() {
		if e := recover(); e != nil {
			frame.Release()
		}
	}()
	if !utils.ChanTryPush(p.frameChannel, frame) {
		frame.Release()
	}
}
