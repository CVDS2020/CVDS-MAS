package media

import (
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/uns"
	"gitee.com/sy_183/sdp"
	"net"
	"strconv"
	"time"
)

type MediaInfo struct {
	Name       string
	Start      time.Time
	End        time.Time
	LocalIP    net.IP
	LocalPort  int
	RemoteIP   net.IP
	RemotePort int
	Transport  string
	Protocol   string
	SSRC       int64
	RtpMap     map[uint8]string
}

func NewMediaInfoFromSDP(content []byte) (*MediaInfo, error) {
	sdpSession, err := sdp.DecodeSession(content, nil)
	if err != nil {
		return nil, err
	}
	sdpMessage := new(sdp.Message)
	sdpDecoder := sdp.NewDecoder(sdpSession)
	if err = sdpDecoder.Decode(sdpMessage); err != nil {
		return nil, err
	}

	var start, end time.Time
	if len(sdpMessage.Timing) > 0 {
		start = sdpMessage.Timing[0].Start
		end = sdpMessage.Timing[0].End
	}

	var remoteIp net.IP
	var remotePort int
	var protocol string
	var transport string
	var sdpMedia *sdp.Media
	for i, m := range sdpMessage.Medias {
		remoteIp = m.Connection.IP
		if m.Description.Type == "video" {
			protocol = m.Description.Protocol
			remotePort = m.Description.Port
			sdpMedia = &sdpMessage.Medias[i]
			break
		}
	}
	switch protocol {
	case "RTP/AVP":
		transport = "udp"
	case "TCP/RTP/AVP":
		transport = "tcp"
	default:
		return nil, errors.NewInvalidArgument("protocol", errors.New("无效的传输协议"))
	}
	if remotePort == 0 {
		return nil, errors.NewNotFound("实时流端口信息")
	}
	if remoteIp == nil {
		remoteIp = sdpMessage.Connection.IP
		if remoteIp == nil {
			return nil, errors.NewNotFound("实时流的IP信息")
		}
	}
	rtpMap := make(map[uint8]string)
	for _, attribute := range sdpMedia.Attributes {
		if attribute.Key == "rtpmap" {
			sdpRtpMap, e := sdp.ParseRtpMap(attribute.Value)
			if e != nil {
				return nil, errors.NewInvalidArgument("media.rtpmap", e)
			}
			if sdpRtpMap.Type > 0 && sdpRtpMap.Type < 128 {
				rtpMap[uint8(sdpRtpMap.Type)] = sdpRtpMap.Format
			}
		}
	}
	if len(rtpMap) == 0 {
		return nil, errors.NewNotFound("实时流格式信息")
	}
	ssrc := int64(-1)
	for _, line := range sdpSession {
		if line.Type == 'y' {
			parsed, e := strconv.ParseUint(uns.BytesToString(line.Value), 10, 32)
			if e != nil {
				return nil, errors.NewInvalidArgument("ssrc", e)
			}
			ssrc = int64(parsed)
		}
	}
	return &MediaInfo{
		Name:       sdpMessage.Name,
		Start:      start,
		End:        end,
		RemoteIP:   remoteIp,
		RemotePort: remotePort,
		Protocol:   protocol,
		Transport:  transport,
		SSRC:       ssrc,
		RtpMap:     rtpMap,
	}, nil
}

func getAddr(ip net.IP, port int, transport string) net.Addr {
	switch transport {
	case "tcp":
		return &net.TCPAddr{
			IP:   ip,
			Port: port,
		}
	case "udp":
		return &net.UDPAddr{
			IP:   ip,
			Port: port,
		}
	default:
		return nil
	}
}

func (m *MediaInfo) LocalAddr(transport string) net.Addr {
	return getAddr(m.LocalIP, m.LocalPort, transport)
}

func (m *MediaInfo) RemoteAddr(transport string) net.Addr {
	return getAddr(m.RemoteIP, m.RemotePort, transport)
}

func (m *MediaInfo) MarshalLogObject(encoder log.ObjectEncoder) error {
	if m.Name != "" {
		encoder.AddString("会话名称", m.Name)
	}
	if m.Start != (time.Time{}) {
		encoder.AddTime("起始时间", m.Start)
	}
	if m.End != (time.Time{}) {
		encoder.AddTime("起始时间", m.End)
	}
	if m.LocalIP != nil {
		encoder.AddString("本地IP", m.LocalIP.String())
	}
	if m.LocalPort != 0 {
		encoder.AddInt("本地端口", m.LocalPort)
	}
	if m.RemoteIP != nil {
		encoder.AddString("远端IP", m.RemoteIP.String())
	}
	if m.RemotePort != 0 {
		encoder.AddInt("远端端口", m.RemotePort)
	}
	if m.Transport == "" {
		encoder.AddString("传输协议", "UNKNOWN")
	} else {
		encoder.AddString("传输协议", m.Transport)
	}
	if m.Protocol != "" {
		encoder.AddString("流模式", m.Protocol)
	}
	if m.SSRC >= 0 {
		encoder.AddInt64("ssrc", m.SSRC)
	}
	if m.RtpMap != nil {
		encoder.AddReflected("rtpMap", m.RtpMap)
	}
	return nil
}
