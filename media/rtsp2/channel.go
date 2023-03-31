package rtsp2

import (
	"bytes"
	"fmt"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/sdp"
	"io"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Channel struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle
	once   atomic.Bool

	server     *Server
	conn       *net.TCPConn
	localAddr  *net.TCPAddr
	remoteAddr *net.TCPAddr
	addrId     string

	parser *Parser

	sessions container.SyncMap[string, *Session]

	log.AtomicLogger
}

func (c *Channel) read() (data []byte, closed bool, err error) {
	buf := make([]byte, 4096)

	n, err := c.conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			return nil, true, nil
		} else if opErr, is := err.(*net.OpError); is && errors.Is(opErr.Err, net.ErrClosed) {
			return nil, true, nil
		}
		return nil, true, c.Logger().ErrorWith("RTSP通道读取错误", err)
	}
	return buf[:n], false, nil
}

func (c *Channel) start(lifecycle.Lifecycle) error {
	if !c.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError("")
	}
	return nil
}

func (c *Channel) run(lifecycle.Lifecycle) error {
	defer func() {
		for _, session := range c.sessions.Values() {
			session.close()
			c.sessions.Delete(session.sessionId)
		}
		c.Logger().Info("RTSP连接已关闭")
	}()

	for {
		data, closed, err := c.read()
		if closed {
			return err
		} else if data == nil {
			continue
		}

		for p := data; len(p) > 0; {
			var ok bool
			if ok, p, err = c.parser.Parse(p); ok {
				switch c.parser.PacketType() {
				case PacketTypeRequest:
					req := c.parser.Request()
					c.Logger().Info("接收到RTSP请求", log.String("请求方法", req.method), log.String("请求URL", req.url.String()))
					now := time.Now()

					go func() (err error) {
						res := NewResponse(req)
						res.header.Add("Date", now.Format("Mon, Jan 02 2006 15:04:05 MST"))

						response400 := func() {
							res.SetStatus(400, "Bad Request")
							c.writeResponse(res)
						}
						response404 := func() {
							res.SetStatus(404, "Not Found")
							c.writeResponse(res)
						}
						response500 := func() {
							res.SetStatus(500, "Internal Server Error")
							c.writeResponse(res)
						}

						switch req.method {
						case "OPTIONS":
							res.header.Add("Public", "DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE, OPTIONS")
							c.writeResponse(res)
						case "DESCRIBE":
							tcpAddr, e := net.ResolveTCPAddr("tcp", req.url.Host)
							if e != nil {
								defer response400()
								return c.Logger().ErrorWith("解析RTSP请求URL主机地址失败", e)
							}
							requestIp := tcpAddr.IP.To4()
							if requestIp == nil {
								defer response400()
								return c.Logger().ErrorWith("解析RTSP请求URL主机地址失败", errors.New("地址无法解析为IPv4地址"))
							}
							path := req.url.Path
							for strings.HasSuffix(path, "/") {
								path = path[:len(path)-1]
							}
							channelId, _ := c.server.pathChannels.Load(path)
							if channelId == "" {
								response404()
								return
							}
							rtpPlayer, e := c.server.mediaManager.GetRtpPlayer(channelId)
							if e != nil {
								response404()
								return e
							}
							streamPlayers := rtpPlayer.StreamPlayers()
							if len(streamPlayers) == 0 {
								defer response404()
								return c.Logger().ErrorWith("获取DESCRIBE失败", errors.New("未找到RTP流信息"))
							}
							sdpMsg := &sdp.Message{
								Version: 0,
								Origin: sdp.Origin{
									Username:       "-",
									SessionID:      int64(sdp.TimeToNTP(now)),
									SessionVersion: int64(sdp.TimeToNTP(now)),
									NetworkType:    "IN",
									AddressType:    "IP4",
									Address:        requestIp.String(),
								},
								Name:  channelId,
								Email: "NONE",
								Attributes: sdp.Attributes{
									{Key: "control", Value: req.url.String()},
								},
								Medias: sdp.Medias{{
									Description: sdp.MediaDescription{
										Type:     "video",
										Protocol: "RTP/AVP",
										Formats:  []string{"96"},
									},
									Connection: sdp.ConnectionData{
										NetworkType: "IN",
										AddressType: "IP4",
									},
									Attributes: sdp.Attributes{
										{Key: "recvonly"},
										{Key: "control", Value: req.url.JoinPath("trackID=1").String()},
										{Key: "rtpmap", Value: "96 H264/90000"},
										{Key: "fmtp", Value: "96 packetization-mode=1"},
									},
								}},
							}
							res.contentType = "application/sdp"
							res.body = append(res.body, sdpMsg.Append(nil).AppendTo(nil))
							c.writeResponse(res)
						case "SETUP":
							sessionId := req.session
							if sessionId == "" {
								path := req.url.Path
								for strings.HasSuffix(path, "/") {
									path = path[:len(path)-1]
								}
								i := strings.LastIndexByte(path, '/')
								if i < 0 {
									defer response400()
									return c.Logger().ErrorWith("解析路径失败", errors.New("错误的路径格式"))
								}
								if path[i+1:] != "trackID=1" {
									defer response400()
									return c.Logger().ErrorWith("解析路径失败", errors.New("错误的路径格式"))
								}
								path = path[:i]

								transport := req.header.Get("Transport")
								tpEntry := strings.Split(transport, ";")
								var clientPort int
								for _, entry := range tpEntry {
									kv := strings.SplitN(strings.TrimSpace(entry), "=", 2)
									if len(kv) == 2 && strings.TrimSpace(kv[0]) == "client_port" {
										portRange := strings.TrimSpace(kv[1])
										if i := strings.IndexByte(portRange, '-'); i >= 0 {
											portRange = portRange[:i]
										}
										parsed, e := strconv.ParseUint(portRange, 10, 16)
										if e != nil {
											response400()
											return e
										}
										clientPort = int(parsed)
									}
								}
								if clientPort == 0 {
									defer response400()
									return c.Logger().ErrorWith("SETUP失败", errors.New("未找到客户端端口"))
								}

								startTimeQuery := req.url.Query().Get("start_time")
								endTimeQuery := req.url.Query().Get("end_time")
								var startTime, endTime time.Time
								var e error
								if startTimeQuery != "" {
									startTime, e = time.ParseInLocation("20060102150405", startTimeQuery, time.FixedZone("Beijing", 8*3600))
									if e != nil {
										defer response400()
										return c.Logger().ErrorWith("SETUP失败", e)
									}
								}
								if endTimeQuery != "" {
									endTime, e = time.ParseInLocation("20060102150405", endTimeQuery, time.FixedZone("Beijing", 8*3600))
									if e != nil {
										defer response400()
										return c.Logger().ErrorWith("SETUP失败", e)
									}
								}

								channelId, _ := c.server.pathChannels.Load(path)
								if channelId == "" {
									response404()
									return
								}

								if startTimeQuery == "" && endTimeQuery == "" {
									_, e = c.server.mediaManager.GetRtpPlayer(channelId)
									if e != nil {
										response404()
										return e
									}
									rtpPusher, e := c.server.mediaManager.OpenRtpPusher(channelId, 0, nil)
									if e != nil {
										response500()
										return e
									}
									defer func() {
										if err != nil {
											rtpPusher.Close(nil)
										}
									}()

									streamPusher, e := rtpPusher.AddStream("udp", c.remoteAddr.IP, clientPort, 0,
										media.NewRtpStreamInfo(&media.MediaTypeH264, nil, 96, 90000))
									if e != nil {
										response500()
										return e
									}
									serverPort := streamPusher.LocalPort()
									ssrc := streamPusher.SSRC()

									transport = fmt.Sprintf("%s;server_port=%d-%d;ssrc=%d", transport, serverPort, serverPort+1, ssrc)

								retry1:
									sessionId = strconv.FormatUint(rand.Uint64(), 10)
									if _, loaded := c.sessions.LoadOrStore(sessionId, &Session{
										sessionId: sessionId,
										rtpPusher: rtpPusher,
									}); loaded {
										goto retry1
									}
								} else {
									historyRtpPusher, e := c.server.mediaManager.OpenHistoryRtpPusher(channelId, startTime.UnixMilli(), endTime.UnixMilli(), nil, nil)
									if e != nil {
										response500()
										return e
									}
									defer func() {
										if err != nil {
											historyRtpPusher.Close(nil)
										}
									}()

									streamPusher, e := historyRtpPusher.AddStream("udp", c.remoteAddr.IP, clientPort, -1,
										media.NewRtpStreamInfo(&media.MediaTypeH264, nil, 96, 90000))
									if e != nil {
										response500()
										return e
									}
									serverPort := streamPusher.LocalPort()
									ssrc := streamPusher.SSRC()

									transport = fmt.Sprintf("%s;server_port=%d-%d;ssrc=%d", transport, serverPort, serverPort+1, ssrc)

								retry2:
									sessionId = strconv.FormatUint(rand.Uint64(), 10)
									if _, loaded := c.sessions.LoadOrStore(sessionId, &Session{
										sessionId:        sessionId,
										historyRtpPusher: historyRtpPusher,
									}); loaded {
										goto retry2
									}
								}

								res.header.Add("Transport", transport)
								res.session = sessionId
								c.writeResponse(res)
							} else {
								defer response404()
								return c.Logger().ErrorWith("SETUP失败", errors.New("此会话已经SETUP完成"))
							}
						case "PLAY":
							sessionId := req.session
							session, _ := c.sessions.Load(sessionId)
							if session == nil {
								defer response404()
								return c.Logger().ErrorWith("PLAY失败", errors.New("未找到对应的session"), log.String("sessionId", sessionId))
							}
							session.play()
							res.session = sessionId
							c.writeResponse(res)
						case "PAUSE":
							sessionId := req.session
							session, _ := c.sessions.Load(sessionId)
							if session == nil {
								defer response404()
								return c.Logger().ErrorWith("PAUSE失败", errors.New("未找到对应的session"), log.String("sessionId", sessionId))
							}
							session.pause()
							res.session = sessionId
							c.writeResponse(res)
						case "TEARDOWN":
							sessionId := req.session
							session, _ := c.sessions.Load(sessionId)
							if session == nil {
								defer response404()
								return c.Logger().ErrorWith("TEARDOWN失败", errors.New("未找到对应的session"), log.String("sessionId", sessionId))
							}
							session.close()
							c.sessions.Delete(sessionId)
							res.session = sessionId
							c.writeResponse(res)
						}
						return nil
					}()
				case PacketTypeResponse:
					// 忽略响应
				case PacketTypeRtp:
					// 忽略RTP
				}
			} else if err != nil {
				switch err.(type) {
				case RtspError:
					c.Logger().ErrorWith("解析RTSP出现错误", err)
					return nil
				case RtpError:
					// 忽略RTP错误
				}
			}
		}
	}
}

func (c *Channel) close(lifecycle.Lifecycle) error {
	if err := c.conn.Close(); err != nil {
		return c.Logger().ErrorWith("关闭RTSP连接失败", err)
	}
	return nil
}

func (c *Channel) writeResponse(response *Response) {
	writeBuffer := bytes.Buffer{}
	response.WriteTo(&writeBuffer)
	if _, err := c.conn.Write(writeBuffer.Bytes()); err != nil {
		c.Logger().ErrorWith("发送RTSP响应失败", err)
	}
}
