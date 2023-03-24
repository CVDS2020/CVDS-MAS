package rtp

import (
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/rtp/rtp"
	rtpServer "gitee.com/sy_183/rtp/server"
	"net"
)

const TCPServerModule = Module + ".tcp-server"

var retryableTCPServer = component.Pointer[lifecycle.Retryable[*rtpServer.TCPServer]]{
	Init: func() *lifecycle.Retryable[*rtpServer.TCPServer] {
		cfg := config.MediaRTPConfig()
		// TCP连接通道配置
		var channelOptions []option.AnyOption
		if buffer := cfg.Buffer; buffer != 0 {
			reversed := cfg.BufferReverse
			if reversed == 0 {
				reversed = rtp.DefaultBufferReverse
			}
			if reversed < 1024 {
				reversed = 1024
			}
			if buffer < reversed {
				buffer = reversed
			}
			var poolProvider pool.PoolProvider[*pool.Buffer]
			switch cfg.BufferPoolType {
			case "slice":
				poolProvider = pool.ProvideSlicePool[*pool.Buffer]
			case "sync":
				poolProvider = pool.ProvideSyncPool[*pool.Buffer]
			case "stack":
				bufferCount := cfg.BufferCount
				if bufferCount == 0 {
					bufferCount = 8
				}
				if bufferCount < 2 {
					bufferCount = 2
				}
				poolProvider = pool.StackPoolProvider[*pool.Buffer](bufferCount)
			default:
				poolProvider = pool.ProvideSlicePool[*pool.Buffer]
			}
			channelOptions = append(channelOptions, rtpServer.WithReadBufferPoolProvider(func() pool.BufferPool {
				return pool.NewDefaultBufferPool(buffer.Uint(), reversed.Uint(), poolProvider)
			}))
		}

		// TCP服务配置
		var serverOptions = []option.AnyOption{
			// 配置接受TCP连接的回调
			rtpServer.WithOnAccept(func(s *rtpServer.TCPServer, conn *net.TCPConn) []option.AnyOption {
				s.Logger().Info("RTP服务接收到新的TCP连接",
					log.String("本端地址", conn.LocalAddr().String()),
					log.String("对端地址", conn.RemoteAddr().String()),
				)
				return nil
			}),
			// 配置TCP连接通道创建成功的回调
			rtpServer.WithOnChannelCreated(func(s *rtpServer.TCPServer, channel *rtpServer.TCPChannel) {
				conn := channel.Conn()
				if cfg.Socket.ReadBuffer != 0 {
					if err := conn.SetReadBuffer(cfg.Socket.ReadBuffer); err != nil {
						channel.Logger().ErrorWith("设置 SOCKET 读缓冲区失败", err, log.Int("缓冲区大小", cfg.Socket.ReadBuffer))
					}
				}
				if cfg.Socket.WriteBuffer != 0 {
					if err := conn.SetWriteBuffer(cfg.Socket.WriteBuffer); err != nil {
						channel.Logger().ErrorWith("设置 SOCKET 写缓冲区失败", err, log.Int("缓冲区大小", cfg.Socket.WriteBuffer))
					}
				}
				if cfg.Socket.Keepalive {
					if err := conn.SetKeepAlive(true); err != nil {
						channel.Logger().ErrorWith("设置TCP连接是否开启 KEEPALIVE 失败", err, log.Bool("是否开启", true))
					}
					if cfg.Socket.KeepalivePeriod != 0 {
						if err := conn.SetKeepAlivePeriod(cfg.Socket.KeepalivePeriod); err != nil {
							channel.Logger().ErrorWith("设置TCP连接 KEEPALIVE 间隔失败", err, log.Duration("KEEPALIVE 间隔", cfg.Socket.KeepalivePeriod))
						}
					}
				}
				if cfg.Socket.DisableNoDelay {
					if err := conn.SetNoDelay(false); err != nil {
						channel.Logger().ErrorWith("设置TCP连接是否开启 NO_DELAY 失败", err, log.Bool("是否开启", false))
					}
				}
				const simpleName = "TCP通道"
				channel.OnClose(defaultOnClose(simpleName, channel)).
					OnClosed(defaultOnClosed(simpleName, channel))
			}),
		}
		if len(channelOptions) > 0 {
			serverOptions = append(serverOptions, rtpServer.WithChannelOptions(channelOptions...))
		}

		// 创建TCP服务
		tcpAddr := &net.TCPAddr{
			IP:   cfg.ListenIPAddr().IP,
			Port: int(cfg.Port),
			Zone: cfg.ListenIPAddr().Zone,
		}
		s := rtpServer.NewTCPServer(tcpAddr, serverOptions...)
		const simpleName = "TCP服务"
		name := fmt.Sprintf("基于TCP的RTP服务(%s)", tcpAddr.String())
		config.InitModuleLogger(s, TCPServerModule, name)
		config.RegisterLoggerConfigReloadedCallback(s, TCPServerModule, name)

		// 配置TCP服务生命周期回调
		s.OnStarting(defaultOnStarting(simpleName, s)).
			OnStarted(defaultOnStarted(simpleName, s)).
			OnClose(defaultOnClose(simpleName, s)).
			OnClosed(defaultOnClosed(simpleName, s))
		return lifecycle.NewRetryable(s)
	},
}

func GetRetryableTCPServer() *lifecycle.Retryable[*rtpServer.TCPServer] {
	return retryableTCPServer.Get()
}

func GetTCPServer() *rtpServer.TCPServer {
	return GetRetryableTCPServer().Get()
}
