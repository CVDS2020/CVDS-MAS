package channel

import (
	"bytes"
	"fmt"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/media"
	rtpFrame "gitee.com/sy_183/rtp/frame"
	"math/rand"
	"net"
	"strings"
	"sync/atomic"
)

type RtpStreamPusher struct {
	id            uint64
	pusher        *RtpPusher
	historyPusher *HistoryRtpPusher
	removed       bool

	transport  string
	localIp    net.IP
	localPort  int
	remoteIp   net.IP
	remotePort int
	ssrc       uint32

	frameWriter *rtpFrame.FullFrameWriter

	tcpConn *net.TCPConn
	udpConn *net.UDPConn
	buffer  bytes.Buffer

	streamInfo atomic.Pointer[media.RtpStreamInfo]

	log.LoggerProvider
}

func newRtpStreamPusher(id uint64, pusher *RtpPusher, historyPusher *HistoryRtpPusher, transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (*RtpStreamPusher, error) {
	if ipv4 := remoteIp.To4(); ipv4 != nil {
		remoteIp = ipv4
	}

	if streamInfo == nil {
		return nil, errors.NewArgumentMissing("streamInfo")
	}

	var tcpConn *net.TCPConn
	var udpConn *net.UDPConn
	var localIp net.IP
	var localPort int
	switch strings.ToLower(transport) {
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
	default:
		return nil, errors.NewInvalidArgument("transport", fmt.Errorf("无效的传输协议(%s)", transport))
	}

	p := &RtpStreamPusher{
		id:            id,
		pusher:        pusher,
		historyPusher: historyPusher,

		transport:  transport,
		localIp:    localIp,
		localPort:  localPort,
		remoteIp:   remoteIp,
		remotePort: remotePort,

		tcpConn: tcpConn,
		udpConn: udpConn,
	}

	p.streamInfo.Store(streamInfo)
	if ssrc < 0 {
		p.ssrc = rand.Uint32()
	} else {
		p.ssrc = uint32(ssrc)
	}

	p.frameWriter = rtpFrame.NewFullFrameWriter(nil)
	p.frameWriter.SetSequenceNumber(0)
	p.frameWriter.SetSSRC(p.ssrc)
	p.frameWriter.SetPayloadType(streamInfo.PayloadType())

	if p.pusher != nil {
		p.LoggerProvider = &p.pusher.AtomicLogger
	} else {
		p.LoggerProvider = &p.historyPusher.AtomicLogger
	}

	return p, nil
}

func (p *RtpStreamPusher) Id() uint64 {
	return p.id
}

func (p *RtpStreamPusher) Pusher() *RtpPusher {
	return p.pusher
}

func (p *RtpStreamPusher) LocalIp() net.IP {
	return p.localIp
}

func (p *RtpStreamPusher) LocalPort() int {
	return p.localPort
}

func (p *RtpStreamPusher) RemoteIp() net.IP {
	return p.remoteIp
}

func (p *RtpStreamPusher) RemotePort() int {
	return p.remotePort
}

func (p *RtpStreamPusher) SSRC() uint32 {
	return p.ssrc
}

func (p *RtpStreamPusher) send(frame rtpFrame.Frame) (err error) {
	defer frame.Release()
	p.frameWriter.SetFrame(frame)
	if p.udpConn != nil {
		if err = p.frameWriter.WriteToUDP(&p.buffer, p.udpConn); err != nil {
			return p.Logger().ErrorWith("发送基于UDP传输的RTP数据失败", err)
		}
	} else {
		defer p.buffer.Reset()
		p.frameWriter.WriteTo(&p.buffer)
		if _, err = p.tcpConn.Write(p.buffer.Bytes()); err != nil {
			return p.Logger().ErrorWith("发送基于TCP传输的RTP数据失败", err)
		}
	}
	return nil
}

func (p *RtpStreamPusher) StreamInfo() *media.RtpStreamInfo {
	return p.streamInfo.Load()
}

func (p *RtpStreamPusher) Info() map[string]any {
	info := map[string]any{
		"id":         p.id,
		"transport":  p.transport,
		"localIp":    p.localIp,
		"localPort":  p.localPort,
		"remoteIp":   p.remoteIp,
		"remotePort": p.remotePort,
		"ssrc":       p.ssrc,
	}
	if streamInfo := p.streamInfo.Load(); streamInfo != nil {
		info["streamInfo"] = map[string]any{
			"mediaType":   streamInfo.MediaType().EncodingName,
			"payloadType": streamInfo.PayloadType(),
			"clockRate":   streamInfo.ClockRate(),
		}
	}
	return info
}

func (p *RtpStreamPusher) close() {
	if p.tcpConn != nil {
		p.tcpConn.Close()
	}
	if p.udpConn != nil {
		p.udpConn.Close()
	}
}
