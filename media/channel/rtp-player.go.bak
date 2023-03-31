package channel

import (
	"errors"
	"gitee.com/sy_183/common/def"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/media"
	mediaRtp "gitee.com/sy_183/cvds-mas/media/rtp"
	rtpFrame "gitee.com/sy_183/rtp/frame"
	rtpServer "gitee.com/sy_183/rtp/server"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type FrameGroup struct {
	frames []rtpFrame.FrameWriter
	start  time.Time
	end    time.Time
	size   int
}

func NewFrameGroup() *FrameGroup {
	return new(FrameGroup)
}

func timeRangeExpandRange(start, end *time.Time, curStart, curEnd time.Time, timeNodeLen int) {
	if start.After(*end) {
		Logger().Fatal("开始时间在结束时间之后", log.Time("开始时间", *start), log.Time("结束时间", *end))
	}
	if curStart.After(curEnd) {
		Logger().Fatal("开始时间在结束时间之后", log.Time("开始时间", curStart), log.Time("结束时间", curEnd))
	}
	if start.IsZero() {
		*start = curStart
		*end = curEnd
	} else {
		du := curStart.Sub(*end)
		if du < 0 {
			guess := time.Duration(0)
			if timeNodeLen > 2 {
				guess = (end.Sub(*start)) / time.Duration(timeNodeLen-2)
			} else if timeNodeLen == 2 {
				guess = curEnd.Sub(curStart)
			}
			offset := du - guess
			*start = start.Add(offset)
		}
		*end = curEnd
	}
}

func (g *FrameGroup) Append(frameWriter rtpFrame.FrameWriter) {
	frame := frameWriter.Frame()
	g.frames = append(g.frames, frameWriter)
	g.size += frameWriter.Size()
	timeRangeExpandRange(&g.start, &g.end, frame.(*rtpFrame.IncomingFrame).Start(), frame.(*rtpFrame.IncomingFrame).End(), len(g.frames))
}

func (g *FrameGroup) Start() time.Time {
	return g.start
}

func (g *FrameGroup) End() time.Time {
	return g.end
}

func (g *FrameGroup) Size() int {
	return g.size
}

func (g *FrameGroup) WriteTo(w io.Writer) (n int64, err error) {
	for _, frameWriter := range g.frames {
		nn, err := frameWriter.WriteTo(w)
		n += nn
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (g *FrameGroup) Clear() {
	for _, frameWriter := range g.frames {
		frameWriter.Frame().Release()
	}
	g.frames = g.frames[:0]
	g.start = time.Time{}
	g.end = time.Time{}
	g.size = 0
}

type RtpPlayerCloseCallback func(player *RtpPlayer, channel *Channel)

type RtpPlayer struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	channel   *Channel
	transport string
	timeout   time.Duration

	rtpManager *rtpServer.Manager
	rtpServer  rtpServer.Server
	stream     atomic.Pointer[rtpServer.Stream]

	localIP    net.IP
	localPort  int
	remoteAddr atomic.Pointer[net.TCPAddr]
	ssrc       atomic.Int64
	mediaMap   atomic.Pointer[[]*media.MediaType]
	mu         sync.Mutex

	storageBufferSize uint
	frameGroup        *FrameGroup

	log.AtomicLogger
}

func NewRTPPlayer(channel *Channel, transport string, timeout time.Duration) (*RtpPlayer, error) {
	timeout = def.SetDefault(timeout, time.Second*5)
	if timeout < time.Second {
		timeout = time.Second
	}
	p := &RtpPlayer{
		channel:           channel,
		timeout:           timeout,
		storageBufferSize: channel.StorageBufferSize(),
		frameGroup:        NewFrameGroup(),
	}
	p.ssrc.Store(-1)

	switch strings.ToLower(transport) {
	case "udp":
		p.transport = "udp"
		rtpManager := mediaRtp.GetManager()
		server := rtpManager.Alloc()
		if server == nil {
			channel.Logger().Error(AllocRTPServerFailed.Error())
			return nil, AllocRTPServerFailed
		}
		p.rtpManager = rtpManager
		p.rtpServer = server
		addr := p.rtpServer.Addr().(*net.UDPAddr)
		if ipv4 := addr.IP.To4(); ipv4 != nil {
			p.localIP = ipv4
		} else {
			p.localIP = addr.IP
		}
		p.localPort = addr.Port
	default:
		p.transport = "tcp"
		p.rtpServer = mediaRtp.GetTCPServer()
		addr := p.rtpServer.Addr().(*net.TCPAddr)
		if ipv4 := addr.IP.To4(); ipv4 != nil {
			p.localIP = ipv4
		} else {
			p.localIP = addr.IP
		}
		p.localPort = addr.Port
	}

	p.SetLogger(channel.Logger().Named(p.DisplayName()))
	p.runner = lifecycle.NewWithInterruptedRun(nil, p.run)
	p.Lifecycle = p.runner
	return p, nil
}

func (p *RtpPlayer) DisplayName() string {
	return p.channel.RtpPlayerDisplayName()
}

func (p *RtpPlayer) Transport() string {
	return p.transport
}

func (p *RtpPlayer) Timeout() time.Duration {
	return p.timeout
}

func (p *RtpPlayer) LocalIP() net.IP {
	return p.localIP
}

func (p *RtpPlayer) LocalPort() int {
	return p.localPort
}

func (p *RtpPlayer) RemoteIPPort() (net.IP, int) {
	remoteTCPAddr := p.remoteAddr.Load()
	if remoteTCPAddr == nil {
		return nil, 0
	}
	return remoteTCPAddr.IP, remoteTCPAddr.Port
}

func (p *RtpPlayer) SSRC() int64 {
	return p.ssrc.Load()
}

func (p *RtpPlayer) Info() map[string]any {
	info := map[string]any{
		"transport": p.Transport(),
		"timeout":   p.Timeout(),
		"localIp":   p.LocalIP(),
		"localPort": p.LocalPort(),
	}
	if ip, port := p.RemoteIPPort(); ip != nil {
		info["remoteIp"] = ip
		info["remotePort"] = port
	}
	if ssrc := p.SSRC(); ssrc >= 0 {
		info["ssrc"] = ssrc
	}
	if mediaMap := p.MediaMap(); mediaMap != nil {
		rtpMap := make(map[uint8]string)
		for id, mediaType := range mediaMap {
			if mediaType != nil {
				rtpMap[uint8(id)] = mediaType.Name
			}
		}
		info["rtpMap"] = rtpMap
	}
	return info
}

func (p *RtpPlayer) getStream() rtpServer.Stream {
	if stream := p.stream.Load(); stream != nil {
		return *stream
	}
	return nil
}

func (p *RtpPlayer) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	defer func() {
		if stream := p.getStream(); stream != nil {
			stream.Close()
		}
		if p.rtpManager != nil {
			p.rtpManager.Free(p.rtpServer)
		}
	}()
	<-interrupter
	return nil
}

func (p *RtpPlayer) storageFrameRaw(frame rtpFrame.Frame, mediaType *media.MediaType) {
	fw := rtpFrame.NewPayloadFrameWriter(frame)
	size := fw.Size()
	if uint(size) > p.storageBufferSize {
		frame.Release()
		return
	}
	if frameGroup := p.frameGroup; uint(frameGroup.Size()+fw.Size()) > p.storageBufferSize {
		if storageChannel := p.channel.loadEnabledStorageChannel(); storageChannel != nil {
			storageChannel.Write(frameGroup, uint(frameGroup.size), frameGroup.Start(), frameGroup.End(), mediaType.ID, true)
		}
		p.frameGroup.Clear()
	}
	p.frameGroup.Append(fw)
}

func (p *RtpPlayer) storageFrameRtp(frame rtpFrame.Frame, mediaType *media.MediaType) {
	fw := rtpFrame.NewFullFrameWriter(frame)
	size := fw.Size()
	if uint(size) > p.storageBufferSize {
		frame.Release()
		return
	}
	if frameGroup := p.frameGroup; uint(frameGroup.Size()+fw.Size()) > p.storageBufferSize {
		if storageChannel := p.channel.loadEnabledStorageChannel(); storageChannel != nil {
			storageChannel.Write(frameGroup, uint(frameGroup.size), frameGroup.Start(), frameGroup.End(), mediaType.ID, true)
		}
		p.frameGroup.Clear()
	}
	p.frameGroup.Append(fw)
}

func (p *RtpPlayer) handleFrame(stream rtpServer.Stream, frame *rtpFrame.IncomingFrame) {
	var mediaType *media.MediaType
	if mediaMap := p.MediaMap(); mediaMap != nil {
		mediaType = mediaMap[frame.PayloadType()]
	}
	if mediaType == nil {
		mediaType = media.GetMediaType(uint32(frame.PayloadType()))
	}
	defer frame.Release()
	if mediaType != nil && mediaType.ID < 128 {
		if uint8(mediaType.ID) != frame.PayloadType() {
			frame.SetPayloadType(uint8(mediaType.ID))
		}
		pushers := p.channel.RtpPushers()
		for _, pusher := range pushers {
			if pusher.Started() {
				pusher.Push(rtpFrame.UseFrame(frame))
			}
		}
		switch p.channel.StorageType() {
		case "rtp":
			p.storageFrameRtp(frame.Use(), mediaType)
		case "raw":
			p.storageFrameRaw(frame.Use(), mediaType)
		default:
			p.storageFrameRtp(frame.Use(), mediaType)
		}
	}
}

func (p *RtpPlayer) onStreamClosed(stream rtpServer.Stream) {
	stream.Logger().Info("RTP流已经关闭")
	lock.LockDo(&p.mu, func() {
		p.stream.Store(nil)
		if storageChannel := p.channel.loadEnabledStorageChannel(); storageChannel != nil {
			storageChannel.Sync()
		}
		p.Close(nil)
	})
}

func (p *RtpPlayer) MediaMap() []*media.MediaType {
	if mediaMap := p.mediaMap.Load(); mediaMap != nil {
		return *mediaMap
	}
	return nil
}

func (p *RtpPlayer) Setup(rtpMap map[uint8]string, remoteIP net.IP, remotePort int, ssrc int64, once bool) error {
	return lock.RLockGet(p.runner, func() error {
		if !p.runner.Running() {
			return p.Logger().ErrorWith("设置RTP拉流参数失败", lifecycle.NewStateNotRunningError(p.DisplayName()))
		}

		mediaMap := make([]*media.MediaType, 128)
		for id, name := range rtpMap {
			if id < 128 {
				mediaMap[id] = media.ParseMediaType(name)
			}
		}

		if ipv4 := remoteIP.To4(); ipv4 != nil {
			remoteIP = ipv4
		}
		var remoteAddr net.Addr
		remoteTCPAddr := &net.TCPAddr{IP: remoteIP, Port: remotePort}
		switch p.transport {
		case "tcp":
			remoteAddr = remoteTCPAddr
		case "udp":
			remoteAddr = (*net.UDPAddr)(remoteTCPAddr)
		}

		return lock.LockGet(&p.mu, func() error {
			//now := time.Now()
			if stream := p.getStream(); stream != nil {
				if once {
					return nil
				}
				//p.setupTime.Store(&now)
				stream.SetRemoteAddr(remoteAddr)
				stream.SetSSRC(ssrc)
			} else {
				p.mediaMap.Store(&mediaMap)
				stream, _ = p.rtpServer.Stream(remoteAddr, ssrc, rtpFrame.NewFrameRTPHandler(rtpFrame.FrameHandlerFunc{
					HandleFrameFn:    p.handleFrame,
					OnStreamClosedFn: p.onStreamClosed,
				}), rtpServer.WithTimeout(p.timeout), rtpServer.WithOnStreamTimeout(func(rtpServer.Stream) {
					stream.Logger().Warn("RTP流超时, 关闭拉流")
				}))
				if stream == nil {
					p.mediaMap.Store(nil)
					return p.Logger().ErrorWith("设置RTP拉流参数失败", errors.New("RTP服务申请流失败"))
				}
				//p.setupTime.Store(&now)
				p.stream.Store(&stream)
			}

			p.remoteAddr.Store(remoteTCPAddr)
			p.ssrc.Store(ssrc)

			fields := []log.Field{log.String("对端IP地址", remoteIP.String()), log.Int("对端端口", remotePort)}
			if remoteAddr != nil {
				fields = append(fields, log.String("对端IP地址", remoteIP.String()), log.Int("对端端口", remotePort))
			}
			if ssrc >= 0 {
				fields = append(fields, log.Int64("SSRC", ssrc))
			}
			if rtpMap != nil {
				fields = append(fields, log.Reflect("RtpMap", rtpMap))
			}
			p.Logger().Info("设置RTP拉流参数成功", fields...)

			return nil
		})
	})
}
