package rtsp

import (
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/rtp/rtp"
	rtpServer "gitee.com/sy_183/rtp/server"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type RtpStream struct {
	session atomic.Pointer[Session]
	channel atomic.Pointer[Channel]
	conn    atomic.Pointer[net.TCPConn]

	rtpStreamId  uint8
	rtcpStreamId uint8
	enableRtcp   bool

	handler     atomic.Pointer[rtpServer.Handler]
	handlerLock sync.Mutex

	ssrc       atomic.Int64
	localAddr  *net.TCPAddr
	remoteAddr atomic.Pointer[net.TCPAddr]

	isInit       bool
	closed       bool
	seq          uint16
	onLossPacket atomic.Pointer[func(stream rtpServer.Stream, loss int)]

	log.AtomicLogger
}

func (s *RtpStream) Session() *Session {
	return s.session.Load()
}

func (s *RtpStream) Channel() *Channel {
	return s.channel.Load()
}

func (s *RtpStream) Conn() *net.TCPConn {
	return s.conn.Load()
}

func (s *RtpStream) RtpStreamId() uint8 {
	return s.rtpStreamId
}

func (s *RtpStream) setOnLossPacket(onLossPacket func(stream rtpServer.Stream, loss int)) {
	s.onLossPacket.Store(&onLossPacket)
}

func (s *RtpStream) Handler() rtpServer.Handler {
	if handler := s.handler.Load(); handler != nil {
		return *handler
	}
	return nil
}

func (s *RtpStream) SetHandler(handler rtpServer.Handler) rtpServer.Stream {
	s.handler.Store(&handler)
	return s
}

func (s *RtpStream) SSRC() int64 {
	return s.ssrc.Load()
}

func (s *RtpStream) SetSSRC(ssrc int64) rtpServer.Stream {
	s.ssrc.Store(ssrc)
	return s
}

func (s *RtpStream) LocalAddr() net.Addr {
	return s.localAddr
}

func (s *RtpStream) RemoteAddr() net.Addr {
	return s.remoteAddr.Load()
}

func (s *RtpStream) SetRemoteAddr(addr net.Addr) rtpServer.Stream {
	if rAddr, is := any(addr).(*net.TCPAddr); is {
		s.remoteAddr.Store(rAddr)
	}
	return s
}

func (s *RtpStream) Timeout() time.Duration {
	return 0
}

func (s *RtpStream) SetTimeout(timeout time.Duration) rtpServer.Stream {
	return s
}

func (s *RtpStream) GetOnTimeout() func(rtpServer.Stream) {
	return nil
}

func (s *RtpStream) SetOnTimeout(onTimeout func(rtpServer.Stream)) rtpServer.Stream {
	return s
}

func (s *RtpStream) OnLossPacket() func(stream rtpServer.Stream, loss int) {
	if onLossPacket := s.onLossPacket.Load(); onLossPacket != nil {
		return *onLossPacket
	}
	return nil
}

func (s *RtpStream) SetOnLossPacket(onLossPacket func(stream rtpServer.Stream, loss int)) rtpServer.Stream {
	s.setOnLossPacket(onLossPacket)
	return s
}

func (s *RtpStream) CloseConn() bool {
	return false
}

func (s *RtpStream) SetCloseConn(enable bool) rtpServer.Stream {
	return s
}

func (s *RtpStream) Write(data []byte) error {
	if conn := s.conn.Load(); conn != nil {
		_, err := conn.Write(data)
		return err
	}
	return net.ErrClosed
}

func (s *RtpStream) Send(layer rtp.Layer) error {
	return nil
}

func (s *RtpStream) Close() {
	//TODO implement me
	panic("implement me")
}

func (s *RtpStream) HandlePacket(stream rtpServer.Stream, packet *rtp.IncomingPacket) (dropped, keep bool) {
	addr, is := packet.Addr().(*net.TCPAddr)
	if !is {
		return true, true
	}
	return lock.LockGetDouble(&s.handlerLock, func() (dropped, keep bool) {
		if s.closed {
			return true, false
		}
		if !s.remoteAddr.CompareAndSwap(nil, addr) {
			ruAddr := s.remoteAddr.Load()
			if !ruAddr.IP.Equal(addr.IP) ||
				ruAddr.Port != addr.Port ||
				ruAddr.Zone != addr.Zone {
				// packet addr not matched, dropped
				packet.Release()
				return true, true
			}
		}
		if !s.ssrc.CompareAndSwap(-1, int64(packet.SSRC())) {
			ssrc := s.ssrc.Load()
			if ssrc != int64(packet.SSRC()) {
				packet.Release()
				return true, true
			}
		}
		// count rtp packet loss
		if !s.isInit {
			s.isInit = true
		} else {
			if diff := packet.SequenceNumber() - s.seq; diff != 1 {
				if onLossPacket := s.OnLossPacket(); onLossPacket != nil {
					onLossPacket(s, int(diff-1))
				}
			}
		}
		s.seq = packet.SequenceNumber()
		if handler := s.Handler(); handler != nil {
			return handler.HandlePacket(stream, packet)
		}
		return true, true
	})
}

func (s *RtpStream) OnParseError(stream rtpServer.Stream, err error) (keep bool) {
	return lock.LockGet(&s.handlerLock, func() bool {
		if s.closed {
			return false
		}
		if handler := s.Handler(); handler != nil {
			return handler.OnParseError(stream, err)
		}
		return true
	})
}

func (s *RtpStream) OnStreamClosed(stream rtpServer.Stream) {
	lock.LockDo(&s.handlerLock, func() {
		if s.closed {
			return
		}
		s.closed = true
		if handler := s.Handler(); handler != nil {
			handler.OnStreamClosed(stream)
		}
	})
}
