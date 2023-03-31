package api

import (
	"context"
	"encoding/json"
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/api/bean"
	"gitee.com/sy_183/cvds-mas/config"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"time"
)

const (
	ServerModule     = Module + ".server"
	ServerModuleName = "API服务"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
	config.Context().RegisterConfigReloadedCallback(func(_, nc *config.Config) {
		if !nc.API.Gin.EnableConsoleColor {
			gin.DisableConsoleColor()
		} else {
			gin.ForceConsoleColor()
		}
	})
}

type Server struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	s *http.Server
	l net.Listener

	sys          *Sys
	mediaChannel *MediaChannel
	//gb28181      *GB28181
	zongHeng *ZongHeng
	rtsp     *RTSP

	log.AtomicLogger
	ginLogger log.AtomicLogger
}

func newServer() *Server {
	s := &Server{
		sys:          GetSys(),
		mediaChannel: GetMediaChannel(),
		rtsp:         GetRTSP(),
	}
	//if gb28181Config := config.GB28181Config(); gb28181Config.Enable {
	//	s.gb28181 = GetGB28181()
	//}
	if zongHengConfig := config.ZongHengConfig(); zongHengConfig.Enable {
		s.zongHeng = GetZongHeng()
	}
	cfg := config.APIConfig()
	s.s = &http.Server{
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}

	s.onConfigReloaded(nil, config.GlobalConfig())
	config.Context().RegisterConfigReloadedCallback(s.onConfigReloaded)

	router := gin.New()
	pprof.Register(router)
	router.Use(gin.LoggerWithFormatter(s.loggerFormatter))
	router.Use(gin.CustomRecoveryWithWriter(nil, s.handleRecovery))
	router.NoRoute(s.handleNoRoute)

	api := router.Group("/api/v1")
	{
		sysAPI := api.Group("/sys")
		{
			sysAPI.POST("/reloadConfig", s.sys.ReloadConfig)
			sysAPI.POST("/restart", s.sys.Restart)
			sysAPI.DELETE("/stop", s.sys.Stop)
		}
		mediaChannelAPI := api.Group("/media/channel")
		{
			mediaChannelAPI.PUT("", s.mediaChannel.Create)
			mediaChannelAPI.GET("", s.mediaChannel.Get)
			mediaChannelAPI.POST("", s.mediaChannel.Modify)
			mediaChannelAPI.DELETE("", s.mediaChannel.Delete)

			mediaChannelAPI.PUT("/openRtpPlayer", s.mediaChannel.OpenRtpPlayer)
			mediaChannelAPI.GET("/getRtpPlayer", s.mediaChannel.GetRtpPlayer)
			mediaChannelAPI.PUT("/rtpPlayerAddStream", s.mediaChannel.RtpPlayerAddStream)
			mediaChannelAPI.POST("/rtpPlayerSetupStream", s.mediaChannel.RtpPlayerSetupStream)
			mediaChannelAPI.DELETE("/rtpPlayerRemoveStream", s.mediaChannel.RtpPlayerRemoveStream)
			mediaChannelAPI.DELETE("/closeRtpPlayer", s.mediaChannel.CloseRtpPlayer)

			mediaChannelAPI.POST("/setupStorage", s.mediaChannel.SetupStorage)
			mediaChannelAPI.POST("/stopStorage", s.mediaChannel.StopStorage)
			mediaChannelAPI.POST("/startRecord", s.mediaChannel.StartRecord)
			mediaChannelAPI.POST("/stopRecord", s.mediaChannel.StopRecord)

			mediaChannelAPI.PUT("/openRtpPusher", s.mediaChannel.OpenRtpPusher)
			mediaChannelAPI.GET("/getRtpPusher", s.mediaChannel.GetRtpPusher)
			mediaChannelAPI.PUT("/rtpPusherAddStream", s.mediaChannel.RtpPusherAddStream)
			mediaChannelAPI.POST("/rtpPusherStartPush", s.mediaChannel.RtpPusherStartPush)
			mediaChannelAPI.POST("/rtpPusherPause", s.mediaChannel.RtpPusherPause)
			mediaChannelAPI.DELETE("/rtpPusherRemoveStream", s.mediaChannel.RtpPusherRemoveStream)
			mediaChannelAPI.DELETE("/closeRtpPusher", s.mediaChannel.CloseRtpPusher)

			mediaChannelAPI.PUT("/openHistoryRtpPusher", s.mediaChannel.OpenHistoryRtpPusher)
			mediaChannelAPI.GET("/getHistoryRtpPusher", s.mediaChannel.GetHistoryRtpPusher)
			mediaChannelAPI.PUT("/historyRtpPusherAddStream", s.mediaChannel.HistoryRtpPusherAddStream)
			mediaChannelAPI.POST("/historyRtpPusherStartPush", s.mediaChannel.HistoryRtpPusherStartPush)
			mediaChannelAPI.POST("/historyRtpPusherPause", s.mediaChannel.HistoryRtpPusherPause)
			mediaChannelAPI.POST("/historyRtpPusherSeek", s.mediaChannel.HistoryRtpPusherSeek)
			mediaChannelAPI.POST("/historyRtpPusherSetScale", s.mediaChannel.HistoryRtpPusherSetScale)
			mediaChannelAPI.DELETE("/historyRtpPusherRemoveStream", s.mediaChannel.HistoryRtpPusherRemoveStream)
			mediaChannelAPI.DELETE("/closeHistoryRtpPusher", s.mediaChannel.CloseHistoryRtpPusher)
		}
		rtspAPI := api.Group("/media/rtsp")
		{
			rtspAPI.PUT("/channel", s.rtsp.AddChannel)
			rtspAPI.GET("/channel", s.rtsp.GetChannel)
			rtspAPI.GET("/channels", s.rtsp.PathChannels)
			rtspAPI.DELETE("/channel", s.rtsp.DeleteChannel)
		}
		//if s.gb28181 != nil {
		//	gb28181API := api.Group("/gb28181")
		//	{
		//		proxy := gb28181API.Group("/proxy")
		//		{
		//			proxy.POST("/invite", s.gb28181.ProxyInvite)
		//			proxy.POST("/ack", s.gb28181.ProxyAck)
		//			proxy.POST("/bye", s.gb28181.ProxyBye)
		//			proxy.POST("/info", s.gb28181.ProxyInfo)
		//		}
		//
		//		storageConfig := gb28181API.Group("/storageConfig")
		//		{
		//			storageConfig.GET("", s.gb28181.GetStorageConfig)
		//			storageConfig.PUT("", s.gb28181.AddStorageConfig)
		//			storageConfig.DELETE("", s.gb28181.DeleteStorageConfig)
		//		}
		//	}
		//}
		if s.zongHeng != nil {
			zongHengAPI := api.Group("/zongheng")
			{
				channelAPI := zongHengAPI.Group("/channel")
				{
					channelAPI.POST("/sync", s.zongHeng.Sync)
				}
			}
		}
	}

	s.s.Handler = router
	s.runner = lifecycle.NewWithRun(s.start, s.run, s.close, lifecycle.WithSelf(s))
	s.Lifecycle = s.runner
	s.OnClosed(func(l lifecycle.Lifecycle, err error) { s.Logger().Info("API服务已关闭") })
	return s
}

func (s *Server) Sys() *Sys {
	return s.sys
}

func (s *Server) MediaChannel() *MediaChannel {
	return s.mediaChannel
}

//func (s *Server) GB28181() *GB28181 {
//	return s.gb28181
//}

func (s *Server) ZongHeng() *ZongHeng {
	return s.zongHeng
}

func (s *Server) loggerFormatter(param gin.LogFormatterParams) string {
	if param.Latency > time.Minute {
		param.Latency = param.Latency.Truncate(time.Second)
	}

	s.ginLogger.Logger().Debug(fmt.Sprintf("接收到%s请求(%s)", param.Method, param.Path),
		log.String("客户端地址", param.ClientIP),
		log.Duration("处理时间", param.Latency),
		log.Int("响应状态码", param.StatusCode),
	)

	return ""
}

func (s *Server) onConfigReloaded(oc, nc *config.Config) {
	config.ReloadModuleLogger(oc, nc, s, ServerModule, ServerModuleName)
	s.ginLogger.SetLogger(s.Logger().WithOptions(log.WithCaller(false)))
}

func (s *Server) handleNoRoute(c *gin.Context) {
	c.AbortWithStatusJSON(404, &bean.Result[any]{
		Code: 404,
		Msg:  "请求资源未找到",
	})
}

func (s *Server) handleRecovery(c *gin.Context, e interface{}) {
	var err any
	jsonErr := func() {
		bytes, je := json.Marshal(e)
		if je == nil {
			err = json.RawMessage(bytes)
		} else {
			err = fmt.Sprint(e)
		}
	}
	switch re := e.(type) {
	case json.Marshaler:
		jsonErr()
	case error:
		err = re.Error()
	default:
		jsonErr()
	}
	c.AbortWithStatusJSON(500, &bean.Result[any]{
		Code: 500,
		Msg:  "服务器处理错误",
		Err:  err,
	})
}

func (s *Server) start(lifecycle.Lifecycle) error {
	addr := config.APIConfig().GetAddr()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return s.Logger().ErrorWith("API服务监听失败", err, log.String("监听地址", addr))
	}
	s.l = l
	s.Logger().Info("API服务监听成功", log.String("监听地址", addr))
	return nil
}

func (s *Server) run(lifecycle.Lifecycle) error {
	tlsConfig := config.APIConfig().TLS
	if tlsConfig.CertFile != "" && tlsConfig.KeyFile != "" {
		if err := s.s.ServeTLS(s.l, tlsConfig.CertFile, tlsConfig.KeyFile); err != nil {
			if err != http.ErrServerClosed {
				return s.Logger().ErrorWith("基于https的API服务出现错误并退出", err)
			}
		}
	} else {
		if err := s.s.Serve(s.l); err != nil {
			if err != http.ErrServerClosed {
				return s.Logger().ErrorWith("基于http的API服务出现错误并退出", err)
			}
		}
	}
	return nil
}

func (s *Server) close(lifecycle.Lifecycle) error {
	return s.s.Shutdown(context.Background())
}

var GetServer = component.NewPointer(newServer).Get
