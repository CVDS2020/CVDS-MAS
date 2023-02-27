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
	ServerModule     = "api.server"
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
	gb28181      *GB28181

	log.AtomicLogger
	ginLogger log.AtomicLogger
}

func newServer() *Server {
	s := &Server{
		sys:          GetSys(),
		mediaChannel: GetMediaChannel(),
		gb28181:      GetGB28181(),
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
		api.POST("/panic", func(c *gin.Context) {
			panic(map[string]any{"a": 1, "b": 2})
		})
		sysAPI := api.Group("/sys")
		{
			sysAPI.POST("/restart", s.sys.Restart)
			sysAPI.DELETE("/stop", s.sys.Stop)
		}
		gb28181API := api.Group("/gb28181")
		{
			proxy := gb28181API.Group("/proxy")
			{
				proxy.POST("/invite", s.gb28181.ProxyInvite)
				proxy.POST("/ack", s.gb28181.ProxyAck)
				proxy.POST("/bye", s.gb28181.ProxyBye)
				proxy.POST("/info", s.gb28181.ProxyInfo)
			}

			storageConfig := gb28181API.Group("/storageConfig")
			{
				storageConfig.GET("", s.gb28181.GetStorageConfig)
				storageConfig.PUT("", s.gb28181.AddStorageConfig)
				storageConfig.DELETE("", s.gb28181.DeleteStorageConfig)
			}
		}
		mediaChannelAPI := api.Group("/media/channel")
		{
			mediaChannelAPI.PUT("", s.mediaChannel.Create)
			mediaChannelAPI.GET("", s.mediaChannel.Get)
			mediaChannelAPI.GET("/list", s.mediaChannel.List)
			mediaChannelAPI.POST("", s.mediaChannel.Modify)
			mediaChannelAPI.DELETE("", s.mediaChannel.Delete)

			mediaChannelAPI.PUT("/openRtpPlayer", s.mediaChannel.OpenRTPPlayer)
			mediaChannelAPI.POST("/setupRtpPlayer", s.mediaChannel.SetupRTPPlayer)
			mediaChannelAPI.DELETE("/closeRtpPlayer", s.mediaChannel.CloseRTPPlayer)

			mediaChannelAPI.PUT("/openRtspPlayer", s.mediaChannel.OpenRTSPPlayer)
			mediaChannelAPI.DELETE("/closeRtspPlayer", s.mediaChannel.CloseRTSPPlayer)

			mediaChannelAPI.PUT("/openRtpPusher", s.mediaChannel.OpenRTPPusher)
			mediaChannelAPI.POST("/startRtpPusher", s.mediaChannel.StartRTPPusher)
			mediaChannelAPI.GET("/getRtpPusher", s.mediaChannel.GetRTPPusher)
			mediaChannelAPI.GET("/listRtpPusher", s.mediaChannel.ListRTPPusher)
			mediaChannelAPI.DELETE("/closeRtpPusher", s.mediaChannel.CloseRTPPusher)

			mediaChannelAPI.POST("/startRecord", s.mediaChannel.StartRecord)
			mediaChannelAPI.POST("/stopRecord", s.mediaChannel.StopRecord)

			mediaChannelAPI.PUT("/openHistoryRtpPusher", s.mediaChannel.OpenHistoryRTPPusher)
			mediaChannelAPI.POST("/startHistoryRtpPusher", s.mediaChannel.StartHistoryRTPPusher)
			mediaChannelAPI.GET("/getHistoryRtpPusher", s.mediaChannel.GetHistoryRTPPusher)
			mediaChannelAPI.GET("/listHistoryRtpPusher", s.mediaChannel.ListHistoryRTPPusher)
			mediaChannelAPI.POST("/pauseHistoryRtpPusher", s.mediaChannel.PauseHistoryRTPPusher)
			mediaChannelAPI.POST("/resumeHistoryRtpPusher", s.mediaChannel.ResumeHistoryRTPPusher)
			mediaChannelAPI.POST("/seekHistoryRtpPusher", s.mediaChannel.SeekHistoryRTPPusher)
			mediaChannelAPI.POST("/setHistoryRtpPusherScale", s.mediaChannel.SetHistoryRTPPusherScale)
			mediaChannelAPI.DELETE("/closeHistoryRtpPusher", s.mediaChannel.CloseHistoryRTPPusher)
		}
	}

	s.s.Handler = router
	s.runner = lifecycle.NewWithRun(s.start, s.run, s.close, lifecycle.WithSelf(s))
	s.Lifecycle = s.runner
	return s
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
	defer s.Logger().Info("API服务已关闭")
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
