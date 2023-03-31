package channel

import (
	"fmt"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/media/rtp"
	rtpFrame "gitee.com/sy_183/rtp/frame"
	rtpServer "gitee.com/sy_183/rtp/server"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SetupConfig struct {
	Timeout             time.Duration `json:"timeout"`
	RtpSecMaxErr        int           `json:"rtpSecMaxErr"`
	RtpMaxSerializedErr int           `json:"rtpMaxSerializedErr"`
}

type rtpKeepChooser struct {
	player      *RtpStreamPlayer
	keepChooser rtpServer.KeepChooser
}

func (c rtpKeepChooser) OnSuccess() {
	c.keepChooser.OnSuccess()
}

func (c rtpKeepChooser) OnError(err error) (keep bool) {
	if !c.keepChooser.OnError(err) {
		c.player.Logger().Error("RTP解析错误超过阈值，关闭RTP流")
		return false
	}
	return true
}

func (c rtpKeepChooser) Reset() {
	c.keepChooser.Reset()
}

type RtpStreamPlayer struct {
	id      uint64
	player  *RtpPlayer
	removed bool

	transport  string
	rtpManager *rtpServer.Manager
	rtpServer  rtpServer.Server
	rtpStream  atomic.Pointer[rtpServer.Stream]

	localIp   net.IP
	localPort int
	mu        sync.Mutex

	streamInfo atomic.Pointer[media.RtpStreamInfo]

	rtpSecMaxErr        atomic.Int64
	rtpMaxSerializedErr atomic.Int64

	droppedFrames     atomic.Uint64
	errorRtpPackets   atomic.Uint64
	droppedRtpPackets atomic.Uint64

	log.LoggerProvider
}

func newRtpStreamPlayer(id uint64, player *RtpPlayer, transport string) (*RtpStreamPlayer, error) {
	p := &RtpStreamPlayer{
		id:             id,
		player:         player,
		LoggerProvider: &player.AtomicLogger,
	}

	switch strings.ToLower(transport) {
	case "udp":
		p.transport = "udp"
		rtpManager := rtp.GetManager()
		server := rtpManager.Alloc()
		if server == nil {
			p.Logger().Error(AllocRTPServerFailed.Error())
			return nil, AllocRTPServerFailed
		}
		p.rtpManager = rtpManager
		p.rtpServer = server
		addr := p.rtpServer.Addr().(*net.UDPAddr)
		if ipv4 := addr.IP.To4(); ipv4 != nil {
			p.localIp = ipv4
		} else {
			p.localIp = addr.IP
		}
		p.localPort = addr.Port
	case "tcp":
		p.transport = "tcp"
		p.rtpServer = rtp.GetTCPServer()
		addr := p.rtpServer.Addr().(*net.TCPAddr)
		if ipv4 := addr.IP.To4(); ipv4 != nil {
			p.localIp = ipv4
		} else {
			p.localIp = addr.IP
		}
		p.localPort = addr.Port
	default:
		return nil, errors.NewInvalidArgument("transport", fmt.Errorf("无效的传输协议(%s)", transport))
	}

	return p, nil
}

func (p *RtpStreamPlayer) getRtpStream() rtpServer.Stream {
	if stream := p.rtpStream.Load(); stream != nil {
		return *stream
	}
	return nil
}

func (p *RtpStreamPlayer) handleRtpFrame(stream rtpServer.Stream, frame *rtpFrame.IncomingFrame) {
	if !p.player.FrameHandler().Push(frame, p.streamInfo.Load()) {
		p.droppedFrames.Add(1)
	}
}

func (p *RtpStreamPlayer) onParseRTPError(stream rtpServer.Stream, err error) (keep bool) {
	p.errorRtpPackets.Add(1)
	return true
}

func (p *RtpStreamPlayer) onRtpStreamClosed(rtpStream rtpServer.Stream) {
	rtpStream.Logger().Info("RTP流已经关闭")
	p.player.removeStream(p.id, false)
}

func (p *RtpStreamPlayer) onRtpStreamTimeout(rtpStream rtpServer.Stream) {
	rtpStream.Logger().Warn("RTP流超时, 关闭拉流")
}

func (p *RtpStreamPlayer) onRtpLossPacket(stream rtpServer.Stream, loss int) {
	p.droppedRtpPackets.Add(uint64(loss))
}

func (p *RtpStreamPlayer) modifyStreamInfo(streamInfo *media.RtpStreamInfo) (old *media.RtpStreamInfo, modified bool) {
	old = p.streamInfo.Load()
	if !streamInfo.Equal(old) {
		p.streamInfo.Store(streamInfo)
		return old, true
	}
	return old, false
}

func (p *RtpStreamPlayer) LocalIp() net.IP {
	return p.localIp
}

func (p *RtpStreamPlayer) LocalPort() int {
	return p.localPort
}

func (p *RtpStreamPlayer) Setup(remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo, config SetupConfig) error {
	if ipv4 := remoteIp.To4(); ipv4 != nil {
		remoteIp = ipv4
	}

	var remoteAddr net.Addr
	remoteTCPAddr := &net.TCPAddr{IP: remoteIp, Port: remotePort}
	switch p.transport {
	case "tcp":
		remoteAddr = remoteTCPAddr
	case "udp":
		remoteAddr = (*net.UDPAddr)(remoteTCPAddr)
	default:
		panic(fmt.Errorf("内部错误: 未知的传输协议(%s)", p.transport))
	}

	return lock.LockGet(&p.mu, func() error {
		if p.removed {
			return p.Logger().ErrorWith("设置RTP拉流参数失败", errors.New("RTP流已被移除"))
		}
		if rtpStream := p.getRtpStream(); rtpStream != nil {
			return p.Logger().ErrorWith("设置RTP拉流参数失败", errors.New("RTP流已经存在"))
		}
		recovery := func() func() {
			oldStreamInfo, modified := p.modifyStreamInfo(streamInfo)
			oldRtpSecMaxErr, oldRtpMaxSerializedErr := p.rtpSecMaxErr.Load(), p.rtpMaxSerializedErr.Load()
			p.rtpSecMaxErr.Store(int64(config.RtpSecMaxErr))
			p.rtpMaxSerializedErr.Store(int64(config.RtpMaxSerializedErr))
			return func() {
				p.rtpMaxSerializedErr.Store(oldRtpMaxSerializedErr)
				p.rtpSecMaxErr.Store(oldRtpSecMaxErr)
				if modified {
					p.streamInfo.Store(oldStreamInfo)
				}
			}
		}()

		rtpStream, _ := p.rtpServer.Stream(remoteAddr, ssrc, rtpServer.KeepChooserHandler(rtpFrame.NewFrameRTPHandler(rtpFrame.FrameHandlerFunc{
			HandleFrameFn:     p.handleRtpFrame,
			OnParseRTPErrorFn: p.onParseRTPError,
			OnStreamClosedFn:  p.onRtpStreamClosed,
		}), rtpKeepChooser{player: p, keepChooser: rtpServer.NewDefaultKeepChooser(config.RtpSecMaxErr, config.RtpMaxSerializedErr, nil)}),
			rtpServer.WithTimeout(config.Timeout),
			rtpServer.WithOnStreamTimeout(p.onRtpStreamTimeout),
			rtpServer.WithOnLossPacket(p.onRtpLossPacket),
		)
		if rtpStream == nil {
			recovery()
			return p.Logger().ErrorWith("设置RTP拉流参数失败", errors.New("RTP服务申请流失败"))
		}
		p.rtpStream.Store(&rtpStream)

		fields := []log.Field{log.String("对端IP地址", remoteIp.String()), log.Int("对端端口", remotePort)}
		if remoteAddr != nil {
			fields = append(fields, log.String("对端IP地址", remoteIp.String()), log.Int("对端端口", remotePort))
		}
		if ssrc >= 0 {
			fields = append(fields, log.Int64("SSRC", ssrc))
		}
		if streamInfo != nil {
			fields = append(fields, log.Object("流信息", streamInfo))
		}
		p.Logger().Info("设置RTP拉流参数成功", fields...)
		return nil
	})
}

func (p *RtpStreamPlayer) StreamInfo() *media.RtpStreamInfo {
	return p.streamInfo.Load()
}

func (p *RtpStreamPlayer) Info() map[string]any {
	m := map[string]any{
		"id":        p.id,
		"transport": p.transport,
	}
	if p.localIp != nil {
		m["localIp"] = p.localIp
		m["localPort"] = p.localPort
	}
	if rtpStream := p.getRtpStream(); rtpStream != nil {
		if remoteAddr := rtpStream.RemoteAddr(); remoteAddr != nil {
			switch addr := remoteAddr.(type) {
			case *net.UDPAddr:
				m["remoteIp"] = addr.IP
				m["remotePort"] = addr.Port
			case *net.TCPAddr:
				m["remoteIp"] = addr.IP
				m["remotePort"] = addr.Port
			}
		}
	}
	if streamInfo := p.streamInfo.Load(); streamInfo != nil {
		m["streamInfo"] = map[string]any{
			"mediaType":   streamInfo.MediaType().EncodingName,
			"payloadType": streamInfo.PayloadType(),
			"clockRate":   streamInfo.ClockRate(),
		}
	}
	return m
}

func (p *RtpStreamPlayer) close(closeStream bool) {
	lock.LockDo(&p.mu, func() {
		p.removed = true
		if rtpStream := p.getRtpStream(); rtpStream != nil && closeStream {
			rtpStream.Close()
		}
		if p.rtpManager != nil {
			p.rtpManager.Free(p.rtpServer)
		}
	})
}

type RtpPlayerCloseCallback func(player *RtpPlayer, channel *Channel)

type RtpPlayer struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle
	once   atomic.Bool

	channel      *Channel
	frameHandler *FrameHandler

	streamPlayerId    atomic.Uint64
	streamPlayerCount atomic.Int64
	streamPlayers     container.SyncMap[uint64, *RtpStreamPlayer]
	streamPlayerLock  sync.Mutex

	log.AtomicLogger
}

func newRtpPlayer(channel *Channel) *RtpPlayer {
	p := &RtpPlayer{
		channel: channel,
	}
	p.frameHandler = NewFrameHandler(p)
	p.SetLogger(channel.Logger().Named(p.DisplayName()))
	p.runner = lifecycle.NewWithInterruptedRun(p.start, p.run)
	p.Lifecycle = p.runner
	return p
}

func (p *RtpPlayer) DisplayName() string {
	return p.channel.RtpPlayerDisplayName()
}

func (p *RtpPlayer) Channel() *Channel {
	return p.channel
}

func (p *RtpPlayer) FrameHandler() *FrameHandler {
	return p.frameHandler
}

func (p *RtpPlayer) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if !p.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError(p.channel.RtpPlayerDisplayName())
	}
	p.frameHandler.Start()
	return nil
}

func (p *RtpPlayer) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	var frameHandlerClosed bool
	defer func() {
		if !frameHandlerClosed {
			p.frameHandler.Shutdown()
		}
		if rtpPushers := p.channel.RtpPushers(); len(rtpPushers) > 0 {
			closedWaiters := make([]<-chan error, 0, len(rtpPushers))
			for _, pusher := range rtpPushers {
				pusher.Close(nil)
				closedWaiters = append(closedWaiters, pusher.ClosedWaiter())
			}
			for _, waiter := range closedWaiters {
				<-waiter
			}
		}
		if streamPlayers := p.streamPlayers.Values(); len(streamPlayers) > 0 {
			for _, streamPlayer := range streamPlayers {
				p.removeStream(streamPlayer.id, true)
			}
		}
	}()
	frameHandlerClosedWaiter := p.frameHandler.ClosedWaiter()
	select {
	case <-frameHandlerClosedWaiter:
		frameHandlerClosed = true
	case <-interrupter:
	}
	return nil
}

func (p *RtpPlayer) StreamPlayers() []*RtpStreamPlayer {
	return p.streamPlayers.Values()
}

func (p *RtpPlayer) AddStream(transport string) (streamPlayer *RtpStreamPlayer, err error) {
	return lock.RLockGetDouble(p.runner, func() (streamPlayer *RtpStreamPlayer, err error) {
		if !p.runner.Running() {
			return nil, p.Logger().ErrorWith("RTP拉流通道添加失败", lifecycle.NewStateNotRunningError(p.DisplayName()))
		}
		streamId := p.streamPlayerId.Add(1)
		streamPlayer, err = newRtpStreamPlayer(streamId, p, transport)
		if err != nil {
			return nil, err
		}
		lock.LockDo(&p.streamPlayerLock, func() {
			p.streamPlayers.Store(streamId, streamPlayer)
			p.streamPlayerCount.Add(1)
		})
		return streamPlayer, nil
	})
}

func (p *RtpPlayer) SetupStream(streamId uint64, remoteIP net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo, config SetupConfig) error {
	return lock.RLockGet(p.runner, func() error {
		if !p.runner.Running() {
			return p.Logger().ErrorWith("RTP拉流通道设置失败", lifecycle.NewStateNotRunningError(p.DisplayName()))
		}
		streamPlayer, ok := p.streamPlayers.Load(streamId)
		if !ok {
			return fmt.Errorf("RTP流(%d)未找到", streamId)
		}
		return streamPlayer.Setup(remoteIP, remotePort, ssrc, streamInfo, config)
	})
}

func (p *RtpPlayer) removeStream(streamId uint64, closeStream bool) {
	streamPlayer, closePlayer := lock.LockGetDouble(&p.streamPlayerLock, func() (*RtpStreamPlayer, bool) {
		streamPlayer, _ := p.streamPlayers.LoadAndDelete(streamId)
		if streamPlayer != nil {
			return streamPlayer, p.streamPlayerCount.Add(-1) == 0
		}
		return nil, false
	})
	if streamPlayer != nil {
		streamPlayer.close(closeStream)
	}
	if closePlayer {
		p.Close(nil)
	}
}

func (p *RtpPlayer) RemoveStream(id uint64) error {
	p.removeStream(id, true)
	return nil
}

func (p *RtpPlayer) Info() map[string]any {
	infos := make([]map[string]any, 0)
	p.streamPlayers.Range(func(id uint64, streamPlayer *RtpStreamPlayer) bool {
		infos = append(infos, streamPlayer.Info())
		return true
	})
	return map[string]any{"streamPlayers": infos}
}
