package rtsp

import (
	"bytes"
	"encoding/binary"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/common/pool"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"gitee.com/sy_183/cvds-mas/storage/file/allocator"
	"gitee.com/sy_183/rtp/rtp"
	rtpServer "gitee.com/sy_183/rtp/server"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var BufferAllocError = errors.New("RTSP读取缓冲区申请失败")

type Channel struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle
	once   atomic.Bool

	server     *Server
	conn       atomic.Pointer[net.TCPConn]
	localAddr  *net.TCPAddr
	remoteAddr *net.TCPAddr
	addrID     string

	sessions     container.SyncMap[string, *Session]
	sessionCount int
	sessionLock  sync.Mutex

	streamAllocator *allocator.BitMap
	streamMap       [256]atomic.Pointer[RtpStream]
	streamLock      sync.Mutex

	readBufferPool pool.BufferPool
	writeBuffer    bytes.Buffer
	writeLock      sync.Mutex

	parser *Parser

	log.AtomicLogger
}

func (c *Channel) Conn() *net.TCPConn {
	return c.conn.Load()
}

func (c *Channel) LocalAddr() *net.TCPAddr {
	return c.localAddr
}

func (c *Channel) RemoteAddr() *net.TCPAddr {
	return c.remoteAddr
}

func (c *Channel) read() (data *pool.Data, closed bool, err error) {
	readBufferPool := c.readBufferPool
	buf := readBufferPool.Get()
	if buf == nil {
		// TCP通道读取缓冲区申请失败，可能是因为流量过大导致，所以可以利用缓冲区做限流
		c.Close(nil)
		return nil, true, c.Logger().ErrorWith("RTSP通道读取错误", BufferAllocError)
	}

	n, err := c.Conn().Read(buf)
	if err != nil {
		if err == io.EOF {
			return nil, true, nil
		} else if opErr, is := err.(*net.OpError); is && errors.Is(opErr.Err, net.ErrClosed) {
			return nil, true, nil
		}
		return nil, true, c.Logger().ErrorWith("RTSP通道读取错误", err)
	}
	return readBufferPool.Alloc(uint(n)), false, nil
}

func (c *Channel) start(lifecycle.Lifecycle) error {
	if !c.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError("")
	}
	return nil
}

func (c *Channel) run(lifecycle.Lifecycle) error {
	defer func() {
		c.conn.Store(nil)
		sessions := lock.LockGet(&c.sessionLock, c.sessions.Values)
		for _, session := range sessions {
			c.removeSession(session)
			if session.Transport() == "udp" {
				c.server.bindSession(session)
			} else {
				session.doOnSessionClosed()
			}
		}
	}()

	rtpKeepChooser := rtpServer.NewDefaultKeepChooser(5, 5, nil)

	for {
		data, closed, err := c.read()
		if closed {
			return err
		} else if data == nil {
			continue
		}

		var chunks []*pool.Data
		for p := data.Data; len(p) > 0; {
			var ok bool
			if ok, p, err = c.parser.Parse(p); ok {
				switch c.parser.PacketType() {
				case PacketTypeRequest:
					request := c.parser.Request()
					go func() {
						response := c.server.handler.HandleRequest(c.server, request)
						buffer := bytes.Buffer{}
						c.WriteResponse(response, &buffer)
						c.Write(buffer.Bytes())
					}()
				case PacketTypeResponse:
					// 忽略响应
				case PacketTypeRtp:
					rtpKeepChooser.OnSuccess()
					packet, streamId, _ := c.parser.RtpPacket()
					packet.SetTime(time.Now())
					packet.SetAddr(c.remoteAddr)
					for _, chunk := range chunks {
						packet.AddRelation(chunk)
					}
					chunks = chunks[:0]
					stream := c.streamMap[streamId].Load()
					if stream != nil {
						_, keep := stream.HandlePacket(stream, packet)
						if !keep {
							if session := stream.Session(); session != nil {
								c.removeRtpStream(session, stream)
							}
						}
					} else {
						packet.Release()
					}
				}
			} else if err != nil {
				switch err.(type) {
				case RtspError:
					c.Logger().ErrorWith("解析RTSP出现错误", err)
					return nil
				case RtpError:
					c.Logger().ErrorWith("解析RTP出现错误", err)
					if !rtpKeepChooser.OnError(err) {
						data.Release()
						return nil
					}
				}
			} else {
				chunks = append(chunks, data.Use())
			}
		}

		data.Release()
	}
}

func (c *Channel) NewSession(sessionId string, transport string) (*Session, error) {
	switch transport = strings.ToLower(transport); transport {
	case "tcp":
	default:
		transport = "udp"
	}
	return lock.RLockGetDouble(c.runner, func() (*Session, error) {
		if !c.runner.Running() {
			return nil, lifecycle.NewStateNotRunningError("")
		}
		session := &Session{
			id:        sessionId,
			server:    c.server,
			transport: transport,
		}
		session.channel.Store(c)
		if err := lock.LockGet(&c.sessionLock, func() error {
			if old, _ := c.sessions.LoadOrStore(sessionId, session); old != nil {
				return errors.New("RTSP会话已经存在")
			}
			c.sessionCount++
			return nil
		}); err != nil {
			return nil, err
		}
		return session, nil
	})
}

func (c *Channel) removeSession(session *Session) (sessionCount int) {
	var streams []*RtpStream
	sessionCount = lock.LockGet(&c.sessionLock, func() int {
		if realSession, _ := c.sessions.Load(session.id); realSession != nil && realSession == session {
			lock.LockDo(&realSession.mu, func() {
				lock.LockDo(&c.streamLock, func() {
					streams = session.rtpStreams.Values()
					for _, stream := range streams {
						c.streamMap[stream.RtpStreamId()].CompareAndSwap(stream, nil)
						stream.session.Store(nil)
						stream.channel.Store(nil)
						stream.conn.Store(nil)
						session.rtpStreams.Delete(stream.rtpStreamId)
					}
				})
				session.channel.Store(nil)
				c.sessions.Delete(session.id)
				c.sessionCount--
			})
		}
		return c.sessionCount
	})
	for _, stream := range streams {
		stream.OnStreamClosed(stream)
	}
	return sessionCount
}

func (c *Channel) RemoveSession(session *Session) (closed bool) {
	if c.removeSession(session) == 0 {
		c.Close(nil)
		return true
	}
	return false
}

func (c *Channel) allocStreamId(id int) int {
	if id < 0 || id > math.MaxUint8 {
		return int(c.streamAllocator.Alloc())
	} else {
		if !c.streamAllocator.AllocIndex(int64(id)) {
			return -1
		}
		return id
	}
}

func (c *Channel) newRtpStream(session *Session, rtpStreamId int, enableRtcp bool, ssrc int64, handler rtpServer.Handler, options ...option.AnyOption) (*RtpStream, error) {
	return lock.RLockGetDouble(c.runner, func() (*RtpStream, error) {
		if !c.runner.Running() {
			return nil, lifecycle.NewStateNotRunningError("")
		}
		return lock.LockGetDouble(&session.mu, func() (*RtpStream, error) {
			if session.channel.Load() != c {
				return nil, errors.New("RTSP会话与当前TCP通道已经分离")
			}
			return lock.LockGetDouble(&c.streamLock, func() (*RtpStream, error) {
				if rtpStreamId = c.allocStreamId(rtpStreamId); rtpStreamId < 0 {
					return nil, errors.New("申请RTP流ID失败")
				}
				var rtcpStreamId int
				if enableRtcp {
					if rtcpStreamId = c.allocStreamId(rtpStreamId + 1); rtcpStreamId < 0 {
						return nil, errors.New("申请RTCP流ID失败")
					}
				}

				rtpStream := &RtpStream{
					rtpStreamId: uint8(rtpStreamId),
					enableRtcp:  enableRtcp,
					localAddr:   c.localAddr,
				}
				if enableRtcp {
					rtpStream.rtcpStreamId = uint8(rtcpStreamId)
				}
				rtpStream.session.Store(session)
				rtpStream.channel.Store(c)
				rtpStream.conn.Store(c.Conn())
				rtpStream.SetRemoteAddr(c.remoteAddr)
				rtpStream.SetSSRC(ssrc)
				rtpStream.SetHandler(handler)
				for _, opt := range options {
					opt.Apply(rtpStream)
				}
				session.rtpStreams.Store(rtpStream.rtpStreamId, rtpStream)
				c.streamMap[rtpStreamId].Store(rtpStream)
				return rtpStream, nil
			})
		})
	})
}

func (c *Channel) removeRtpStream(session *Session, stream *RtpStream) {
	if lock.LockGet(&session.mu, func() bool {
		if session.channel.Load() != c {
			return false
		}
		return lock.LockGet(&c.streamLock, func() bool {
			if c.streamMap[stream.RtpStreamId()].CompareAndSwap(stream, nil) {
				stream.session.Store(nil)
				stream.channel.Store(nil)
				stream.conn.Store(nil)
				session.rtpStreams.Delete(stream.rtpStreamId)
				return true
			}
			return false
		})
	}) {
		stream.OnStreamClosed(stream)
	}
}

func (c *Channel) Write(data []byte) (err error) {
	if conn := c.Conn(); conn != nil {
		_, err = conn.Write(data)
		return
	}
	return net.ErrClosed
}

func (c *Channel) WriteRtp(streamId uint8, layer rtp.Layer, w io.Writer) (n int64, err error) {
	prefix := [4]byte{}
	prefix[0] = '$'
	prefix[1] = streamId
	size := layer.Size()
	if size > math.MaxUint16 {
		return 0, errors.NewSizeOutOfRange("RTP包", 0, math.MaxUint16, int64(size), true)
	}
	binary.BigEndian.PutUint16(prefix[2:], uint16(size))
	if err = ioUtils.Write(w, prefix[:], &n); err != nil {
		return
	}
	err = ioUtils.WriteTo(layer, w, &n)
	return
}

func (c *Channel) WriteResponse(response *Response, w io.Writer) (n int64, err error) {
	err = ioUtils.WriteTo(response, w, &n)
	return
}
