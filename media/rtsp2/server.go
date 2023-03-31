package rtsp2

import (
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/id"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/media"
	mediaChannel "gitee.com/sy_183/cvds-mas/media/channel"
	"net"
	"sync"
	"sync/atomic"
)

const (
	Module     = media.Module + ".rtsp-server"
	ModuleName = "RTSP服务"
)

type Server struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle
	once   atomic.Bool

	mediaManager *mediaChannel.Manager

	addr     *net.TCPAddr
	listener *net.TCPListener

	channels map[string]*Channel
	mu       sync.Mutex

	pathChannels container.SyncMap[string, string]

	log.AtomicLogger
}

func NewServer() *Server {
	s := &Server{
		mediaManager: mediaChannel.GetManager(),
		addr:         &net.TCPAddr{Port: 554},
		channels:     make(map[string]*Channel),
	}
	config.InitModuleLogger(s, Module, ModuleName)
	config.RegisterLoggerConfigReloadedCallback(s, Module, ModuleName)
	s.runner = lifecycle.NewWithRun(s.start, s.run, s.close)
	s.Lifecycle = s.runner
	return s
}

func (s *Server) newChannel(conn *net.TCPConn) *Channel {
	localTCPAddr := conn.LocalAddr().(*net.TCPAddr)
	remoteTCPAddr := conn.RemoteAddr().(*net.TCPAddr)
	ch := &Channel{
		server:     s,
		conn:       conn,
		localAddr:  localTCPAddr,
		remoteAddr: remoteTCPAddr,
		addrId:     id.GenerateTcpAddrId(remoteTCPAddr),

		parser: NewParser(0, 0, 0),
	}
	ch.SetLogger(s.Logger().Named(fmt.Sprintf("RTSP通道(%s <--> %s)", localTCPAddr, remoteTCPAddr)))
	ch.runner = lifecycle.NewWithRun(ch.start, ch.run, ch.close)
	ch.Lifecycle = ch.runner
	return ch
}

func (s *Server) accept() (conn *net.TCPConn, closed bool, err error) {
	conn, err = s.listener.AcceptTCP()
	if err != nil {
		if opErr, is := err.(*net.OpError); is && errors.Is(opErr.Err, net.ErrClosed) {
			return nil, true, nil
		}
		return nil, true, s.Logger().ErrorWith("RTSP服务接受TCP连接失败", err)
	}
	s.Logger().Info("RTSP服务接受到新的TCP连接", log.String("远端地址", conn.RemoteAddr().String()))
	return
}

func (s *Server) start(lifecycle.Lifecycle) error {
	if !s.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError("")
	}
	listener, err := net.ListenTCP("tcp", s.addr)
	if err != nil {
		return s.Logger().ErrorWith("RTSP服务监听失败", err)
	}
	s.Logger().Info("RTSP服务监听成功", log.String("监听地址", s.addr.String()))
	s.listener = listener
	return nil
}

func (s *Server) run(lifecycle.Lifecycle) error {
	defer func() {
		lock.LockDo(&s.mu, func() {
			for _, channel := range s.channels {
				channel.Close(nil)
			}
		})
	}()

	for {
		conn, closed, err := s.accept()
		if closed {
			return err
		} else if conn == nil {
			continue
		}

		ch := s.newChannel(conn)

		lock.LockDo(&s.mu, func() {
			// 添加通道到TCP服务中
			s.channels[ch.addrId] = ch
		})

		// 当通道关闭时，从TCP服务中删除此通道
		ch.OnClosed(func(lifecycle.Lifecycle, error) {
			lock.LockDo(&s.mu, func() {
				if s.channels[ch.addrId] == ch {
					delete(s.channels, ch.addrId)
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

func (s *Server) AddChannel(path string, channel string) {
	s.pathChannels.Store(path, channel)
}

func (s *Server) GetChannel(path string) string {
	channel, _ := s.pathChannels.Load(path)
	return channel
}

func (s *Server) PathChannels() map[string]string {
	return s.pathChannels.Map()
}

func (s *Server) DeleteChannel(path string) {
	s.pathChannels.Delete(path)
}

var GetServer = component.NewPointer(NewServer).Get
