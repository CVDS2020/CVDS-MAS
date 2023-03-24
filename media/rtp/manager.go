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

const (
	ManagerModule     = Module + ".manager"
	ManagerModuleName = "RTP服务管理器"
	UDPServerModule   = Module + ".udp-server"
)

var manager = component.Pointer[rtpServer.Manager]{
	Init: func() *rtpServer.Manager {
		// UDP服务配置
		cfg := config.MediaRTPConfig()
		var serverOptions []option.AnyOption
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
			serverOptions = append(serverOptions, rtpServer.WithReadBufferPoolProvider(func() pool.BufferPool {
				return pool.NewDefaultBufferPool(buffer.Uint(), reversed.Uint(), poolProvider)
			}))
		}

		// UDP服务管理器配置
		var managerOptions = []rtpServer.ManagerOption{
			rtpServer.WithPortRange(cfg.PortRange.Start, cfg.PortRange.End, cfg.PortRange.Excludes...),
			rtpServer.WithServerMaxUsed(1),
		}
		for _, port := range cfg.Ports {
			managerOptions = append(managerOptions, rtpServer.WithPort(port.RTP, port.RTCP))
		}
		if len(serverOptions) > 0 {
			managerOptions = append(managerOptions, rtpServer.WithServerOptions(serverOptions...))
		}
		if cfg.RetryInterval != 0 {
			managerOptions = append(managerOptions, rtpServer.WithServerRestartInterval(cfg.RetryInterval))
		}

		// 创建UDP服务管理器
		manager := rtpServer.NewManager(cfg.ListenIPAddr(), func(m *rtpServer.Manager, port uint16, options ...option.AnyOption) rtpServer.Server {
			// 创建UDP服务
			ipAddr := m.Addr()
			udpAddr := &net.UDPAddr{
				IP:   ipAddr.IP,
				Port: int(port),
				Zone: ipAddr.Zone,
			}
			s := rtpServer.NewUDPServer(udpAddr, options...)

			const (
				loggerConfigReloadedCallbackId = "loggerConfigReloadedCallbackId"
				simpleName                     = "UDP服务"
			)
			name := fmt.Sprintf("基于UDP的RTP服务(%s)", udpAddr.String())
			// 配置UDP服务生命周期回调
			s.OnStarting(func(lifecycle.Lifecycle) {
				config.InitModuleLogger(s, UDPServerModule, name)
				s.SetField(loggerConfigReloadedCallbackId, config.RegisterLoggerConfigReloadedCallback(s, UDPServerModule, name))
				logOnStarting(simpleName, s)
			})
			s.OnStarted(func(lifecycle lifecycle.Lifecycle, err error) {
				listener := s.Listener()
				if cfg.Socket.ReadBuffer != 0 {
					if err := listener.SetReadBuffer(cfg.Socket.ReadBuffer); err != nil {
						s.Logger().ErrorWith("设置 SOCKET 读缓冲区失败", err, log.Int("缓冲区大小", cfg.Socket.ReadBuffer))
					}
				}
				if cfg.Socket.WriteBuffer != 0 {
					if err := listener.SetWriteBuffer(cfg.Socket.WriteBuffer); err != nil {
						s.Logger().ErrorWith("设置 SOCKET 写缓冲区失败", err, log.Int("缓冲区大小", cfg.Socket.WriteBuffer))
					}
				}
				logOnStarted(simpleName, s, err)
			})
			s.OnClose(defaultOnClose(simpleName, s))
			s.OnClosed(func(_ lifecycle.Lifecycle, err error) {
				logOnClosed(simpleName, s, err)
				config.UnregisterLoggerConfigReloadedCallback(s.Field(loggerConfigReloadedCallbackId).(uint64))
			})
			return s
		}, managerOptions...)
		config.InitModuleLogger(manager, ManagerModule, ManagerModuleName)
		config.RegisterLoggerConfigReloadedCallback(manager, ManagerModule, ManagerModuleName)
		return manager
	},
}

func GetManager() *rtpServer.Manager {
	return manager.Get()
}
