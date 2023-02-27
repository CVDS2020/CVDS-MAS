package channel

import (
	"errors"
	"gitee.com/sy_183/common/def"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/media"
	mediaRtp "gitee.com/sy_183/cvds-mas/media/rtp"
	"gitee.com/sy_183/rtp/frame"
	rtpServer "gitee.com/sy_183/rtp/server"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Frame struct {
	*frame.Frame
	mediaType   uint32
	storageType uint32
}

func (f Frame) WriteTo(w io.Writer) (n int64, err error) {
	return frame.FullLayerWriter{Layer: f.Layer}.WriteTo(w)
}

func (f Frame) Size() uint {
	return frame.FullLayerWriter{Layer: f.Layer}.Size()
}

func (f Frame) MediaType() uint32 {
	return f.mediaType
}

func (f Frame) StorageType() uint32 {
	return f.storageType
}

type FrameInfo struct {
	Timestamp   uint32
	PayloadType uint8
	Start       time.Time
	End         time.Time
}

type RTPPlayerCloseCallback func(player *RTPPlayer, channel *Channel)

type RTPPlayer struct {
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

	curFrame  atomic.Pointer[FrameInfo]
	setupTime atomic.Pointer[time.Time]

	log.AtomicLogger
}

func NewRTPPlayer(channel *Channel, transport string, timeout time.Duration) (*RTPPlayer, error) {
	timeout = def.SetDefault(timeout, time.Second*5)
	if timeout < time.Second {
		timeout = time.Second
	}
	p := &RTPPlayer{
		channel: channel,
		timeout: timeout,
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

func (p *RTPPlayer) DisplayName() string {
	return p.channel.RTPPlayerDisplayName()
}

func (p *RTPPlayer) Transport() string {
	return p.transport
}

func (p *RTPPlayer) Timeout() time.Duration {
	return p.timeout
}

func (p *RTPPlayer) LocalIP() net.IP {
	return p.localIP
}

func (p *RTPPlayer) LocalPort() int {
	return p.localPort
}

func (p *RTPPlayer) RemoteIPPort() (net.IP, int) {
	remoteTCPAddr := p.remoteAddr.Load()
	if remoteTCPAddr == nil {
		return nil, 0
	}
	return remoteTCPAddr.IP, remoteTCPAddr.Port
}

func (p *RTPPlayer) SSRC() int64 {
	return p.ssrc.Load()
}

func (p *RTPPlayer) Info() map[string]any {
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
				rtpMap[uint8(id)] = mediaType.UpperName
			}
		}
		info["rtpMap"] = rtpMap
	}
	return info
}

func (p *RTPPlayer) getStream() rtpServer.Stream {
	if stream := p.stream.Load(); stream != nil {
		return *stream
	}
	return nil
}

func (p *RTPPlayer) getSetupTime() time.Time {
	if tp := p.setupTime.Load(); tp != nil {
		return *tp
	}
	return time.Time{}
}

func (p *RTPPlayer) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	ticker := time.NewTicker(p.timeout / 5)
	defer func() {
		if stream := p.getStream(); stream != nil {
			stream.Close()
		}
		if p.rtpManager != nil {
			p.rtpManager.Free(p.rtpServer)
		}
	}()

	for {
		select {
		case now := <-ticker.C:
			curFrame := p.curFrame.Load()
			if stream := p.getStream(); stream != nil {
				var du time.Duration
				if curFrame == nil {
					du = now.Sub(p.getSetupTime())
				} else {
					du = now.Sub(curFrame.End)
				}
				if du > p.timeout {
					p.Logger().Warn("一段时间内没有接收到任何帧, 关闭拉流", log.Duration("时间", du))
					p.Close(nil)
					<-interrupter
					return nil
				}
			}
		case <-interrupter:
			return nil
		}
	}
}

func (p *RTPPlayer) handleFrame(stream rtpServer.Stream, f *frame.Frame) {
	var mediaType *media.MediaType
	if mediaMap := p.MediaMap(); mediaMap != nil {
		mediaType = mediaMap[f.PayloadType]
	}
	if mediaType == nil {
		mediaType = media.GetMediaType(uint32(f.PayloadType))
	}
	if mediaType != nil && mediaType.ID < 128 {
		if uint8(mediaType.ID) != f.PayloadType {
			for _, layer := range f.RTPLayers {
				layer.SetPayloadType(uint8(mediaType.ID))
			}
			f.PayloadType = uint8(mediaType.ID)
		}
		p.curFrame.Store(&FrameInfo{
			Timestamp:   f.Timestamp,
			PayloadType: f.PayloadType,
			Start:       f.Start,
			End:         f.End,
		})
		if storageChannel := p.channel.loadEnabledStorageChannel(); storageChannel != nil {
			storageChannel.Write(Frame{
				Frame:       f,
				mediaType:   mediaType.ID,
				storageType: p.channel.StorageType().ID,
			})
			return
		}
	}
	f.Release()
}

func (p *RTPPlayer) onStreamClosed(stream rtpServer.Stream) {
	stream.Logger().Info("RTP流已经关闭")
	lock.LockDo(&p.mu, func() {
		p.stream.Store(nil)
		if storageChannel := p.channel.loadEnabledStorageChannel(); storageChannel != nil {
			storageChannel.Sync()
		}
	})
}

func (p *RTPPlayer) MediaMap() []*media.MediaType {
	if mediaMap := p.mediaMap.Load(); mediaMap != nil {
		return *mediaMap
	}
	return nil
}

func (p *RTPPlayer) Setup(rtpMap map[uint8]string, remoteIP net.IP, remotePort int, ssrc int64, once bool) error {
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
			now := time.Now()
			if stream := p.getStream(); stream != nil {
				if once {
					return nil
				}
				p.setupTime.Store(&now)
				stream.SetRemoteAddr(remoteAddr)
				stream.SetSSRC(ssrc)
			} else {
				p.mediaMap.Store(&mediaMap)
				stream, _ = p.rtpServer.Stream(remoteAddr, ssrc, frame.NewFrameRTPHandler(frame.FrameHandlerFunc{
					HandleFrameFn:    p.handleFrame,
					OnStreamClosedFn: p.onStreamClosed,
				}))
				if stream == nil {
					p.mediaMap.Store(nil)
					return p.Logger().ErrorWith("设置RTP拉流参数失败", errors.New("RTP服务申请流失败"))
				}
				p.setupTime.Store(&now)
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
