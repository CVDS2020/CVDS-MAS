package rtsp

import (
	"errors"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/id"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"net"
	"sync"
	"sync/atomic"
)

type Server struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle
	once   atomic.Bool

	addr     *net.TCPAddr
	listener *net.TCPListener

	channelOptions []Option
	channels       map[string]*Channel
	mu             sync.Mutex

	session     container.SyncMap[string, *Session]
	sessionLock sync.Mutex

	// 接受TCP连接的处理函数
	onAccept atomic.Pointer[func(s *Server, conn *net.TCPConn) []Option]
	// tCP连接通道创建成功的处理函数
	onChannelCreated atomic.Pointer[func(s *Server, channel *Channel)]

	handler Handler

	log.AtomicLogger
}

func newServer(listener *net.TCPListener, addr *net.TCPAddr, options ...Option) *Server {
	var s *Server
	if listener != nil {
		s = &Server{addr: listener.Addr().(*net.TCPAddr)}
		s.listener = listener
	} else {
		s = &Server{addr: addr}
	}
	s.channels = make(map[string]*Channel)
	for _, option := range options {
		option.apply(s)
	}
	if s.addr == nil {
		s.addr = &net.TCPAddr{IP: net.IP{0, 0, 0, 0}, Port: 554}
	}
	s.runner = lifecycle.NewWithRun(s.start, s.run, s.close)
	s.Lifecycle = s.runner
	return s
}

func (s *Server) setOnAccept(onAccept func(s *Server, conn *net.TCPConn) []Option) {
	s.onAccept.Store(&onAccept)
}

func (s *Server) setOnChannelCreated(onChannelCreated func(s *Server, channel *Channel)) {
	s.onChannelCreated.Store(&onChannelCreated)
}

func (s *Server) GetOnAccept() func(s *Server, conn *net.TCPConn) []Option {
	if onAccept := s.onAccept.Load(); onAccept != nil {
		return *onAccept
	}
	return nil
}

func (s *Server) SetOnAccept(onAccept func(s *Server, conn *net.TCPConn) []Option) *Server {
	s.setOnAccept(onAccept)
	return s
}

func (s *Server) GetOnChannelCreated() func(s *Server, channel *Channel) {
	if onChannelCreated := s.onChannelCreated.Load(); onChannelCreated != nil {
		return *onChannelCreated
	}
	return nil
}

func (s *Server) SetOnChannelCreated(onChannelCreated func(s *Server, channel *Channel)) *Server {
	s.setOnChannelCreated(onChannelCreated)
	return s
}

func (s *Server) accept() (conn *net.TCPConn, closed bool, err error) {
	conn, err = s.listener.AcceptTCP()
	if err != nil {
		if opErr, is := err.(*net.OpError); is && errors.Is(opErr.Err, net.ErrClosed) {
			return nil, true, nil
		}
		return nil, true, s.Logger().ErrorWith("RTSP服务接受TCP连接失败", err)
	}
	return
}

func (s *Server) newChannel(conn *net.TCPConn, options ...Option) *Channel {
	remoteTCPAddr := conn.RemoteAddr().(*net.TCPAddr)
	var localTCPAddr *net.TCPAddr
	if localAddr := conn.LocalAddr(); localAddr != nil {
		localTCPAddr = localAddr.(*net.TCPAddr)
	}
	addrID := id.GenerateTcpAddrId(remoteTCPAddr)
	ch := &Channel{
		server: s,

		localAddr:  localTCPAddr,
		remoteAddr: remoteTCPAddr,
		addrID:     addrID,
	}
	ch.conn.Store(conn)
	for _, option := range options {
		option.apply(ch)
	}

	return ch
}

func (s *Server) start(lifecycle.Lifecycle) error {
	if !s.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError("")
	}
	if s.listener == nil {
		listener, err := net.ListenTCP("tcp", s.addr)
		if err != nil {
			return s.Logger().ErrorWith("RTSP服务监听失败", err)
		}
		s.listener = listener
	}
	return nil
}

func (s *Server) run(lifecycle.Lifecycle) error {
	defer func() {
	}()

	for {
		conn, closed, err := s.accept()
		if closed {
			return err
		} else if conn == nil {
			continue
		}

		options := s.channelOptions
		if onAccept := s.GetOnAccept(); onAccept != nil {
			options = append(options, onAccept(s, conn)...)
		}
		ch := s.newChannel(conn, options...)

		lock.LockDo(&s.mu, func() {
			// 添加通道到TCP服务中
			s.channels[ch.addrID] = ch
		})

		// 当通道关闭时，从TCP服务中删除此通道
		ch.OnClosed(func(lifecycle.Lifecycle, error) {
			lock.LockDo(&s.mu, func() {
				if s.channels[ch.addrID] == ch {
					delete(s.channels, ch.addrID)
				}
			})
		})
		// 启动通道，一定可以启动成功，因为通道刚刚创建一定是第一次启动
		ch.Start()
	}
}

func (s *Server) close(lifecycle.Lifecycle) error {
	if err := s.listener.Close(); err != nil {
		return s.Logger().ErrorWith("RTSP服务关闭失败", err)
	}
	return nil
}

func (s *Server) bindSession(session *Session) bool {
	return lock.LockGet(&s.sessionLock, func() bool {
		if !s.runner.Running() {
			return false
		}
		if old, _ := s.session.LoadOrStore(session.id, session); old != nil {
			return false
		}
		return true
	})
}
