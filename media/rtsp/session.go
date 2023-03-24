package rtsp

import (
	"errors"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/option"
	rtpServer "gitee.com/sy_183/rtp/server"
	"sync"
	"sync/atomic"
)

type Session struct {
	id      string
	channel atomic.Pointer[Channel]
	server  *Server
	closed  atomic.Bool

	transport string

	rtpStreams container.SyncMap[uint8, *RtpStream]
	mu         sync.Mutex

	onSessionClosed func(session *Session)
}

func (s *Session) Id() string {
	return s.id
}

func (s *Session) Channel() *Channel {
	return s.channel.Load()
}

func (s *Session) Server() *Server {
	return s.server
}

func (s *Session) Transport() string {
	return s.transport
}

func (s *Session) NewRtpStream(rtpStreamId int, enableRtcp bool, ssrc int64, handler rtpServer.Handler, options ...option.AnyOption) (*RtpStream, error) {
	if ch := s.channel.Load(); ch != nil {
		return ch.newRtpStream(s, rtpStreamId, enableRtcp, ssrc, handler, options...)
	}
	return nil, errors.New("RTSP会话与当前TCP通道已经分离")
}

func (s *Session) doOnSessionClosed() {
	if s.closed.CompareAndSwap(false, true) {
		if s.onSessionClosed != nil {
			s.onSessionClosed(s)
		}
	}
}
