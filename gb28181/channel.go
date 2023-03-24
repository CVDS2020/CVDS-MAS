package gb28181

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/def"
	errors "gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/uns"
	"gitee.com/sy_183/cvds-mas/api/bean"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/db"
	gbErrors "gitee.com/sy_183/cvds-mas/gb28181/errors"
	modelPkg "gitee.com/sy_183/cvds-mas/gb28181/model"
	"gitee.com/sy_183/cvds-mas/gb28181/sip"
	responsePkg "gitee.com/sy_183/cvds-mas/gb28181/sip/response"
	"gitee.com/sy_183/cvds-mas/media"
	mediaChannel "gitee.com/sy_183/cvds-mas/media/channel"
	"gitee.com/sy_183/sdp"
	"gorm.io/gorm"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ChannelModule = Module + ".channel"

type Channel struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	name string

	dbManager            *db.DBManager
	storageConfigManager *StorageConfigManager
	mcManager            *mediaChannel.Manager
	mediaChannel         *mediaChannel.Channel
	playSignal           chan struct{}

	proxyConfig *config.GB28181Proxy

	storageSetup  bool
	recordEnabled bool

	mediaTransport string
	mediaProtocol  string

	channel *modelPkg.Channel

	stream       *modelPkg.Stream
	streamLoaded bool
	streamActive bool
	streamLock   sync.Mutex

	playbackDialogs container.SyncMap[string, *PlaybackDialog]

	storageConfig *modelPkg.StorageConfig
	gateway       *modelPkg.Gateway

	log.AtomicLogger
}

type MediaInfo struct {
	Name       string
	Start      time.Time
	End        time.Time
	RemoteIP   net.IP
	RemotePort int
	Protocol   string
	Transport  string
	SSRC       int64
	RtpMap     map[uint8]string
}

type NPT struct {
	Now bool
	NPT float64
}

type MANSRTSP struct {
	Method string
	Scale  float64
	Range  *NPT
}

func (r *MANSRTSP) Parse(raw []byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	if scanner.Scan() {
		requestLine := scanner.Text()
		if i := strings.IndexByte(requestLine, ' '); i > 0 {
			method := requestLine[:i]
			switch upMethod := strings.ToUpper(method); method {
			case "PLAY", "PAUSE", "TEARDOWN":
				r.Method = upMethod
			default:
				return errors.NewInvalidArgument("MANSRTSP.method", fmt.Errorf("无效的MANSRTSP方法(%s)", method))
			}
		} else {
			return errors.NewInvalidArgument("MANSRTSP.requestLine", errors.New("MANSRTSP请求行格式错误"))
		}
	}

	r.Scale = 0
	for scanner.Scan() {
		header := scanner.Text()
		if tokens := strings.SplitN(header, ":", 2); len(tokens) == 2 {
			name, value := strings.TrimSpace(tokens[0]), strings.TrimSpace(tokens[1])
			switch strings.ToLower(name) {
			case "scale":
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return errors.NewInvalidArgument("MANSRTSP.scale", err)
				}
				r.Scale = parsed
			case "range":
				if strings.HasPrefix(strings.ToLower(value), "npt=") {
					npt := value[4:]
					if i := strings.IndexByte(npt, '-'); i > 0 {
						if npt[:i] == "now" {
							r.Range = &NPT{Now: true}
							continue
						}
						parsed, err := strconv.ParseFloat(npt[:i], 64)
						if err != nil {
							return errors.NewInvalidArgument("MANSRTSP.range.npt", err)
						}
						r.Range = &NPT{NPT: parsed}
					} else {
						return errors.NewInvalidArgument("MANSRTSP.range.npt", fmt.Errorf("无效的NPT范围(%s)", npt))
					}
				} else {
					return errors.NewInvalidArgument("MANSRTSP.range", fmt.Errorf("无效的range(%s)", value))
				}
			}
		}
	}

	return nil
}

type PlaybackDialog struct {
	channel *Channel
	Pusher  *mediaChannel.HistoryRtpPusher

	DialogId string
	CallId   string
	FromTag  string
	ToTag    string
	CSeq     uint32

	RemoteDisplayName string
	RemoteId          string
	RemoteIp          string
	RemotePort        int
	RemoteDomain      string
	Transport         string

	byeOnce sync.Once
}

func newPlaybackDialog(channel *Channel, request *sip.Request) (*PlaybackDialog, error) {
	if len(request.Via) == 0 {
		return nil, errors.NewArgumentMissing("request.via")
	}

	var contactURI string
	var displayName string
	if request.Contact != nil {
		contactURI = request.Contact.Address.URI
		displayName = request.Contact.Address.DisplayName
	}

	if contactURI == "" {
		return nil, errors.NewArgumentMissing("request.contact")
	}

	uri := sip.URI{}
	if err := uri.Parse(contactURI); err != nil {
		return nil, errors.NewInvalidArgument("request.contact.address.uri", err)
	}

	dialog := &PlaybackDialog{
		channel: channel,

		CallId:  request.CallId,
		FromTag: request.From.Tag,
		ToTag:   request.To.Tag,
		CSeq:    request.CSeq,

		RemoteDisplayName: displayName,
		RemoteId:          uri.User,
		RemoteIp:          uri.Host,
		RemotePort:        uri.Port,
		Transport:         request.Via[0].Transport,
	}

	if dialog.ToTag == "" {
		dialog.ToTag = sip.CreateTag()
	}

	dialog.DialogId = sip.CreateDialogId(dialog.CallId, dialog.FromTag, dialog.ToTag)
	return dialog, nil
}

func (d *PlaybackDialog) createByeRequest() *sip.Request {
	proxyConfig := d.channel.ProxyConfig()
	return &sip.Request{
		URI: sip.CreateSipURI(d.RemoteId, d.RemoteIp, d.RemotePort, ""),
		Message: sip.Message{
			LocalIp:   proxyConfig.SipIp,
			LocalPort: proxyConfig.SipPort,
			From: sip.From{
				Address: sip.Address{
					DisplayName: proxyConfig.DisplayName,
					URI:         sip.CreateSipURI(proxyConfig.Id, proxyConfig.SipIp, proxyConfig.SipPort, proxyConfig.SipDomain),
				},
				Tag: d.FromTag,
			},
			To: sip.To{
				Address: sip.Address{
					DisplayName: d.RemoteDisplayName,
					URI:         sip.CreateSipURI(d.RemoteId, d.RemoteIp, d.RemotePort, d.RemoteDomain),
				},
				Tag: d.ToTag,
			},
			Via: []sip.Via{{
				Host:      proxyConfig.SipIp,
				Port:      def.SetDefault(proxyConfig.SipPort, 5060),
				Transport: d.Transport,
				Branch:    sip.CreateBranch(),
				RPort:     true,
			}},
			CSeq:        d.CSeq + 1,
			CallId:      d.CallId,
			MaxForwards: 70,
			Contact: &sip.Contact{
				Address: sip.Address{
					DisplayName: proxyConfig.DisplayName,
					URI:         sip.CreateSipURI(proxyConfig.Id, proxyConfig.SipIp, proxyConfig.SipPort, ""),
				},
			},
		},
	}
}

func newChannel(channel *modelPkg.Channel, manager *ChannelManager) *Channel {
	c := &Channel{
		name:                 channel.Name,
		dbManager:            GetDBManager(),
		storageConfigManager: GetStorageConfigManager(),
		mcManager:            mediaChannel.GetManager(),
		playSignal:           make(chan struct{}, 1),
		proxyConfig:          config.GB28181ProxyConfig(),
		channel:              channel,
	}
	c.SetLogger(manager.Logger().Named(c.DisplayName()))
	c.runner = lifecycle.NewWithInterruptedRun(c.start, c.run)
	c.Lifecycle = c.runner
	return c
}

func (c *Channel) Name() string {
	return c.name
}

func (c *Channel) Model() *modelPkg.Channel {
	return c.channel
}

func (c *Channel) DisplayName() string {
	return fmt.Sprintf("国标通道(%s)", c.name)
}

func (c *Channel) ProxyConfig() *config.GB28181Proxy {
	return c.proxyConfig
}

func (c *Channel) sipTransport(transport string) string {
	switch strings.ToLower(transport) {
	case "tcp":
		return "tcp"
	default:
		return "udp"
	}
}

func (c *Channel) createSSRC() string {
	return c.formatSSRC(int64(rand.Uint32()))
}

func (c *Channel) formatSSRC(ssrc int64) string {
	if ssrc < 0 {
		return ""
	}
	return fmt.Sprintf("%010d", ssrc)
}

func (c *Channel) enabledRtpMaps() (rtpMap []*sdp.RtpMap) {
	mediaConfig := config.MediaConfig()
	for _, typ := range mediaConfig.EnabledMediaTypes {
		mediaType := media.ParseMediaType(typ)
		if mediaType != nil && mediaType.PT < 128 {
			rtpMap = append(rtpMap, sdp.NewRtpMap(int(mediaType.PT), mediaType.EncodingName, mediaType.ClockRate))
		}
	}
	if len(rtpMap) == 0 {
		rtpMap = append(rtpMap, sdp.NewRtpMap(int(media.MediaTypePS.PT), media.MediaTypePS.EncodingName, media.MediaTypePS.ClockRate))
	}
	return
}

func (c *Channel) createRequestSDP(name string, start, end time.Time, mediaIp net.IP, mediaPort int, protocol string, ssrc string) sdp.Session {
	if ipv4 := mediaIp.To4(); ipv4 != nil {
		mediaIp = ipv4
	}
	addressType := "IP4"
	if len(mediaIp) == net.IPv6len {
		addressType = "IP6"
	}
	rtpMaps := c.enabledRtpMaps()
	sdpMedia := sdp.Media{
		Description: sdp.MediaDescription{
			Type:     "video",
			Port:     mediaPort,
			Protocol: protocol,
		},
		Attributes: sdp.Attributes{{Key: "recvonly"}},
	}
	for _, rtpMap := range rtpMaps {
		sdpMedia.Description.Formats = append(sdpMedia.Description.Formats, strconv.Itoa(rtpMap.Type))
		sdpMedia.Attributes = append(sdpMedia.Attributes, sdp.Attribute{
			Key:   "rtpmap",
			Value: rtpMap.String(),
		})
	}
	proxyConfig := c.ProxyConfig()
	message := &sdp.Message{
		Version: 0,
		Origin: sdp.Origin{
			Username:       proxyConfig.Id,
			SessionID:      int64(sdp.TimeToNTP(time.Now())),
			SessionVersion: int64(sdp.TimeToNTP(time.Now())),
			NetworkType:    "IN",
			AddressType:    addressType,
			Address:        mediaIp.String(),
		},
		Name: name,
		Connection: sdp.ConnectionData{
			NetworkType: "IN",
			AddressType: addressType,
			IP:          mediaIp,
		},
		Timing: []sdp.Timing{{Start: start, End: end}},
		Medias: sdp.Medias{sdpMedia},
	}
	if ssrc != "" {
		return append(message.Append(nil), sdp.Line{Type: 'y', Value: uns.StringToBytes(ssrc)})
	}
	return message.Append(nil)
}

func (c *Channel) createResponseSDP(name string, start, end time.Time, mediaIp net.IP, mediaPort int, protocol string, ssrc string, rtpMap *sdp.RtpMap) sdp.Session {
	if ipv4 := mediaIp.To4(); ipv4 != nil {
		mediaIp = ipv4
	}
	addressType := "IP4"
	if len(mediaIp) == net.IPv6len {
		addressType = "IP6"
	}
	proxyConfig := c.ProxyConfig()
	message := &sdp.Message{
		Version: 0,
		Origin: sdp.Origin{
			Username:       proxyConfig.Id,
			SessionID:      int64(sdp.TimeToNTP(time.Now())),
			SessionVersion: int64(sdp.TimeToNTP(time.Now())),
			NetworkType:    "IN",
			AddressType:    addressType,
			Address:        mediaIp.String(),
		},
		Name: name,
		Connection: sdp.ConnectionData{
			NetworkType: "IN",
			AddressType: addressType,
			IP:          mediaIp,
		},
		Timing: []sdp.Timing{{Start: start, End: end}},
		Medias: sdp.Medias{{
			Description: sdp.MediaDescription{
				Type:     "video",
				Port:     mediaPort,
				Protocol: protocol,
				Formats:  []string{strconv.Itoa(rtpMap.Type)},
			},
			Attributes: sdp.Attributes{
				{Key: "sendonly"},
				{Key: "rtpmap", Value: rtpMap.String()},
			},
		}},
	}
	if ssrc != "" {
		return append(message.Append(nil), sdp.Line{Type: 'y', Value: uns.StringToBytes(ssrc)})
	}
	return message.Append(nil)
}

func (c *Channel) createInviteRequest(ip string, port int, domain string, transport string, ssrc string, content string) *sip.Request {
	channel := c.channel
	proxyConfig := c.proxyConfig
	return &sip.Request{
		URI: sip.CreateSipURI(channel.ChannelId, ip, port, ""),
		Message: sip.Message{
			LocalIp:   proxyConfig.SipIp,
			LocalPort: proxyConfig.SipPort,
			From: sip.From{
				Address: sip.Address{
					DisplayName: proxyConfig.DisplayName,
					URI:         sip.CreateSipURI(proxyConfig.Id, proxyConfig.SipIp, proxyConfig.SipPort, proxyConfig.SipDomain),
				},
				Tag: sip.CreateTag(),
			},
			To: sip.To{
				Address: sip.Address{
					DisplayName: channel.DisplayName,
					URI:         sip.CreateSipURI(channel.ChannelId, ip, port, domain),
				},
			},
			Via: []sip.Via{{
				Host:      proxyConfig.SipIp,
				Port:      def.SetDefault(proxyConfig.SipPort, 5060),
				Transport: transport,
				Branch:    sip.CreateBranch(),
				RPort:     true,
			}},
			CSeq:        1,
			CallId:      sip.CreateCallId(proxyConfig.SipIp, proxyConfig.SipPort),
			MaxForwards: 70,
			Contact: &sip.Contact{
				Address: sip.Address{
					DisplayName: proxyConfig.DisplayName,
					URI:         sip.CreateSipURI(proxyConfig.Id, proxyConfig.SipIp, proxyConfig.SipPort, ""),
				},
			},
			Subject:     fmt.Sprintf("%s:%s,%s:0", channel.ChannelId, ssrc, proxyConfig.Id),
			ContentType: "application/sdp",
			Content:     content,
		},
	}
}

func (c *Channel) createByeRequest(stream *modelPkg.Stream) *sip.Request {
	proxyConfig := c.proxyConfig
	return &sip.Request{
		URI: sip.CreateSipURI(stream.RemoteId, stream.RemoteIp, int(stream.RemotePort), ""),
		Message: sip.Message{
			LocalIp:   proxyConfig.SipIp,
			LocalPort: proxyConfig.SipPort,
			From: sip.From{
				Address: sip.Address{
					DisplayName: proxyConfig.DisplayName,
					URI:         sip.CreateSipURI(proxyConfig.Id, proxyConfig.SipIp, proxyConfig.SipPort, proxyConfig.SipDomain),
				},
				Tag: stream.FromTag,
			},
			To: sip.To{
				Address: sip.Address{
					DisplayName: stream.RemoteDisplayName,
					URI:         sip.CreateSipURI(stream.RemoteId, stream.RemoteIp, int(stream.RemotePort), stream.RemoteDomain),
				},
				Tag: stream.ToTag,
			},
			Via: []sip.Via{{
				Host:      proxyConfig.SipIp,
				Port:      def.SetDefault(proxyConfig.SipPort, 5060),
				Transport: stream.Transport,
				Branch:    sip.CreateBranch(),
				RPort:     true,
			}},
			CSeq:        stream.CSeq + 1,
			CallId:      stream.CallId,
			MaxForwards: 70,
			Contact: &sip.Contact{
				Address: sip.Address{
					DisplayName: proxyConfig.DisplayName,
					URI:         sip.CreateSipURI(proxyConfig.Id, proxyConfig.SipIp, proxyConfig.SipPort, ""),
				},
			},
		},
	}
}

func (c *Channel) createResponse(request *sip.Request, statusCode int, toTag string) *sip.Response {
	response := &sip.Response{
		StatusCode:   statusCode,
		ReasonPhrase: responsePkg.ReasonPhrase(statusCode),
		Message: sip.Message{
			LocalIp:   request.LocalIp,
			LocalPort: request.LocalPort,
			From:      request.From,
			To:        request.To,
			Via:       append([]sip.Via(nil), request.Via...),
			CSeq:      request.CSeq,
			CallId:    request.CallId,
		},
	}
	if response.To.Tag == "" {
		if toTag != "" {
			response.To.Tag = toTag
		} else {
			response.To.Tag = sip.CreateTag()
		}
	}
	return response
}

func (c *Channel) getCallInfo(action string) (gateway *modelPkg.Gateway, callIp string, callPort int, callDomain string, transport string, err error) {
	channel := c.channel
	if channel.Gateway != "" {
		model := new(modelPkg.Gateway)
		if res := c.dbManager.Table(modelPkg.GatewayTableName).Where("name = ?", channel.Gateway).First(model); res.Error != nil {
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				err = c.Logger().ErrorWith(action+"失败", &errors.NotFound{Target: "网关"},
					log.String("通道", channel.Name),
					log.String("网关", channel.Gateway),
				)
				return
			}
			err = c.Logger().ErrorWith("查找通道对应的网关失败", res.Error,
				log.String("通道", channel.Name),
				log.String("网关", channel.Gateway),
			)
			return
		}
		gateway = model
		if gateway.GatewayIp == "" {
			err = c.Logger().ErrorWith(action+"失败", errors.NewArgumentMissing("gateway.gatewayIp"),
				log.String("网关", gateway.Name))
			return
		}
		callIp = gateway.GatewayIp
		callPort = int(gateway.GatewayPort)
		callDomain = gateway.GatewayDomain
	} else if channel.ChannelIp != "" {
		callIp = channel.ChannelIp
		callPort = int(channel.ChannelPort)
		callDomain = channel.ChannelDomain
	} else {
		err = c.Logger().ErrorWith(action+"失败",
			errors.NewArgumentMissingOne("channel.channelIp", "channel.gateway"), log.String("通道", channel.Name))
		return
	}
	transport = "udp"
	if channel.Transport != "" {
		transport = c.sipTransport(channel.Transport)
	} else if gateway != nil && gateway.Transport != "" {
		transport = c.sipTransport(gateway.Transport)
	}
	return
}

func (c *Channel) sendRequest(method string, sipRequest *sip.Request) (*sip.Response, error) {
	url := c.proxyConfig.HttpUrl
	if strings.HasSuffix(url, "/") {
		url += method
	} else {
		url += "/" + method
	}
	content, err := json.MarshalIndent(sipRequest, "", "  ")
	if err != nil {
		return nil, c.Logger().ErrorWith("SIP请求对象编码失败", err)
	}
	response, err := http.Post(url, "application/json", bytes.NewReader(content))
	if err != nil {
		return nil, c.Logger().ErrorWith("发送HTTP请求失败", err, log.String("url", url))
	}
	content, err = io.ReadAll(response.Body)
	if err != nil {
		return nil, c.Logger().ErrorWith("读取HTTP响应结果失败", err)
	}
	res := bean.Result[*sip.Response]{}
	if err := json.Unmarshal(content, &res); err != nil {
		return nil, c.Logger().ErrorWith("解析HTTP响应结果失败", err)
	}
	if res.Code != 200 {
		var fields []log.Field
		if res.Err != nil {
			fields = append(fields, log.Reflect("错误详细信息", res.Err))
		}
		return nil, c.Logger().ErrorWith("HTTP响应失败", &errors.HttpResponseError{Code: res.Code, Msg: res.Msg}, fields...)
	}
	c.Logger().Info("基于HTTP协议的SIP请求成功",
		log.String("SIP请求方法", strings.ToUpper(method)),
		log.String("SIP请求URI", sipRequest.URI),
		log.Int("SIP响应码", res.Data.StatusCode),
	)
	return res.Data, nil
}

func (c *Channel) parseMessageSDP(message *sip.Message) (*media.MediaInfo, error) {
	if message.ContentType == "" || message.Content == "" {
		return nil, c.Logger().ErrorWith("获取SIP消息体失败",
			errors.NewArgumentMissing("message.contentType", "message.content"))
	}
	if strings.ToLower(message.ContentType) != "application/sdp" {
		return nil, c.Logger().ErrorWith("获取SIP消息体失败",
			errors.NewInvalidArgument("message.contentType", errors.New("响应类型必须为application/sdp")),
			log.String("响应类型", message.ContentType))
	}

	sdpSession, err := sdp.DecodeSession(uns.StringToBytes(message.Content), nil)
	if err != nil {
		return nil, c.Logger().ErrorWith("解析SDP失败", err)
	}
	sdpMessage := new(sdp.Message)
	sdpDecoder := sdp.NewDecoder(sdpSession)
	if err = sdpDecoder.Decode(sdpMessage); err != nil {
		return nil, c.Logger().ErrorWith("解析SDP失败", err)
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
	switch protocol = strings.ToUpper(protocol); protocol {
	case "RTP/AVP":
		transport = "udp"
	case "TCP/RTP/AVP":
		transport = "tcp"
	default:
		return nil, c.Logger().ErrorWith("解析SDP中的传输协议失败", errors.NewInvalidArgument("protocol", errors.New("无效的传输协议")))
	}
	if remotePort == 0 {
		return nil, c.Logger().ErrorWith("获取SDP媒体信息失败", &errors.NotFound{Target: "实时视频流的端口信息"})
	}
	if remoteIp == nil {
		remoteIp = sdpMessage.Connection.IP
		if remoteIp == nil {
			return nil, c.Logger().ErrorWith("获取SDP媒体信息失败", &errors.NotFound{Target: "实时视频流的IP信息"})
		}
	}
	rtpMap := make(map[uint8]string)
	for _, attribute := range sdpMedia.Attributes {
		if attribute.Key == "rtpmap" {
			sdpRtpMap, e := sdp.ParseRtpMap(attribute.Value)
			if e != nil {
				return nil, c.Logger().ErrorWith("解析SDP中的rtpmap失败", e)
			}
			if sdpRtpMap.Type > 0 && sdpRtpMap.Type < 128 {
				rtpMap[uint8(sdpRtpMap.Type)] = sdpRtpMap.Format
			}
		}
	}
	if len(rtpMap) == 0 {
		return nil, c.Logger().ErrorWith("获取SDP媒体信息失败", &errors.NotFound{Target: "实时视频流的格式信息"})
	}
	ssrc := int64(-1)
	for _, line := range sdpSession {
		if line.Type == 'y' {
			parsed, e := strconv.ParseUint(uns.BytesToString(line.Value), 10, 32)
			if e != nil {
				return nil, c.Logger().ErrorWith("解析SDP中的SSRC失败", e)
			}
			ssrc = int64(parsed)
		}
	}
	return &media.MediaInfo{
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

func (c *Channel) parseMessageMANSRTSP(message *sip.Message) (*MANSRTSP, error) {
	if message.ContentType == "" || message.Content == "" {
		return nil, c.Logger().ErrorWith("获取SIP消息体失败",
			errors.NewArgumentMissing("message.contentType", "message.content"))
	}
	if strings.ToLower(message.ContentType) != "application/mansrtsp" {
		return nil, c.Logger().ErrorWith("获取SIP消息体失败",
			errors.NewInvalidArgument("message.contentType", errors.New("响应类型必须为application/sdp")),
			log.String("响应类型", message.ContentType))
	}

	rtsp := new(MANSRTSP)
	if err := rtsp.Parse(uns.StringToBytes(message.Content)); err != nil {
		return nil, err
	}

	return rtsp, nil
}

func (c *Channel) checkStream(stream *modelPkg.Stream) error {
	if stream.CallId == "" ||
		stream.FromTag == "" ||
		stream.ToTag == "" ||
		stream.RemoteId == "" ||
		stream.RemoteIp == "" ||
		stream.Transport == "" {
		return errors.NewArgumentMissing(
			"stream.callId",
			"stream.fromTag",
			"stream.toTag",
			"stream.remoteId",
			"stream.remoteIp",
			"stream.transport",
		)
	}
	switch strings.ToLower(stream.Transport) {
	case "udp", "tcp":
	default:
		return errors.NewInvalidArgument("stream.transport", fmt.Errorf("无效的SIP传输协议(%s)", stream.Transport))
	}
	return nil
}

func (c *Channel) createStream(response *sip.Response, remoteId string, remoteIp string, remotePort int, remoteDomain string, transport string) (*modelPkg.Stream, error) {
	if response.CallId == "" || response.From.Tag == "" || response.To.Tag == "" {
		return nil, c.Logger().ErrorWith("创建通道实时流失败",
			errors.NewArgumentMissing("response.callId", "response.from.tag", "response.to.tag"))
	}
	if response.CSeq != 1 {
		return nil, c.Logger().ErrorWith("创建通道实时流失败",
			errors.NewInvalidArgument("response.cSeq", errors.New("SIP响应CSeq与请求不匹配")),
			log.Uint32("请求CSeq", 1),
			log.Uint32("响应CSeq", response.CSeq),
		)
	}

	var contactURI string
	var displayName string
	if contact := response.Contact; contact != nil {
		contactURI = contact.Address.URI
		displayName = contact.Address.DisplayName
	}

	if contactURI != "" {
		url := sip.URI{}
		if err := url.Parse(contactURI); err != nil {
			return nil, c.Logger().ErrorWith("创建通道实时流失败", errors.NewInvalidArgument("response.contact.address.uri", err))
		}
		remoteId = url.User
		remoteIp = url.Host
		remotePort = url.Port
	}
	c.Logger().Info("创建通道实时流成功")
	return &modelPkg.Stream{
		Channel:           c.name,
		CallId:            response.CallId,
		FromTag:           response.From.Tag,
		ToTag:             response.To.Tag,
		RemoteDisplayName: displayName,
		RemoteId:          remoteId,
		RemoteIp:          remoteIp,
		RemotePort:        int32(remotePort),
		RemoteDomain:      remoteDomain,
		Transport:         transport,
		CSeq:              1,
	}, nil
}

func (c *Channel) doCloseStream(stream *modelPkg.Stream) error {
	byeRequest := c.createByeRequest(stream)
	response, err := c.sendRequest("bye", byeRequest)
	if err != nil {
		return err
	}
	if response.StatusCode != responsePkg.Ok && response.StatusCode != responsePkg.CallOrTransactionDoesNotExist {
		c.Logger().ErrorWith("关闭通道实时流失败",
			&gbErrors.SipResponseError{StatusCode: response.StatusCode, ReasonPhrase: response.ReasonPhrase})
		return err
	}
	c.Logger().Info("关闭通道实时流成功")
	return nil
}

func (c *Channel) deleteStream(stream *modelPkg.Stream) error {
	if res := c.dbManager.Table(modelPkg.StreamTableName).Delete(stream); res.Error != nil {
		return c.Logger().ErrorWith("从数据库中删除流信息失败", res.Error)
	}
	lock.LockDo(&c.streamLock, func() { c.stream = nil })
	return nil
}

func (c *Channel) closeStream() (err error) {
	var stream *modelPkg.Stream
	var streamActive bool
	if !c.streamLoaded {
		model := new(modelPkg.Stream)
		if res := c.dbManager.Table(modelPkg.StreamTableName).Where("channel = ?", c.name).First(model); res.Error != nil {
			if !errors.Is(res.Error, gorm.ErrRecordNotFound) {
				return c.Logger().ErrorWith("查找通道流失败", res.Error, log.String("通道", c.name))
			} else {
				// 从数据库中未找到通道对应的流，不需要执行任何操作
				c.streamLoaded = true
				return nil
			}
		}
		c.streamLoaded = true
		stream = model

		if err := c.checkStream(stream); err != nil {
			c.Logger().ErrorWith("检查通道实时流信息失败", err)
			lock.LockDo(&c.streamLock, func() { c.stream = stream })
			return c.deleteStream(stream)
		}

		streamActive = true
		lock.LockDo(&c.streamLock, func() {
			c.stream = model
			c.streamActive = true
		})
	} else {
		stream, streamActive = lock.LockGetDouble(&c.streamLock, func() (*modelPkg.Stream, bool) {
			return c.stream, c.streamActive
		})
	}

	if stream != nil {
		if !streamActive {
			// 流已经关闭，但未从数据库中删除
			return c.deleteStream(stream)
		}

		// 存在未关闭的流，使用BYE方法关闭实时流
		if err := c.doCloseStream(stream); err != nil {
			return err
		}

		stream = lock.LockGet(&c.streamLock, func() *modelPkg.Stream {
			if c.streamActive {
				c.streamActive = false
			}
			return c.stream
		})

		if stream != nil {
			return c.deleteStream(stream)
		}
	}
	return nil
}

func (c *Channel) play(channel *modelPkg.Channel, checkStream bool) (err error) {
	// 获取点播对端SIP信息
	_, callIp, callPort, callDomain, transport, err := c.getCallInfo("初始化通道")
	if err != nil {
		return err
	}

	// 检查是否已经存在实时流，如果存在则先关闭
	if checkStream {
		if err := c.closeStream(); err != nil {
			return err
		}
	}

	// 打开RTP拉流服务(获取本地的RTP服务IP和端口)
	player, err := c.mediaChannel.OpenRtpPlayer(c.mediaTransport, time.Second*5, nil)
	if err != nil {
		return err
	}
	mediaLocalIp := config.MediaRTPConfig().GetLocalIP()
	mediaLocalPort := player.LocalPort()

	defer func() {
		// RTP拉流服务已启动，出现错误需要关闭RTP拉流
		if err != nil {
			c.mediaChannel.CloseRtpPlayer()
		}
	}()

	// 构建SDP会话协商消息
	requestSSRC := c.createSSRC()
	sdpSession := c.createRequestSDP("Play", time.Time{}, time.Time{}, mediaLocalIp, mediaLocalPort, c.mediaProtocol, requestSSRC)
	sdpContent := sdpSession.AppendTo(nil)

	// 发送SIP INVITE请求，获取INVITE响应并检查是否合法
	response, err := c.sendRequest("invite",
		c.createInviteRequest(callIp, callPort, callDomain, transport, requestSSRC, uns.BytesToString(sdpContent)))
	if err != nil {
		return err
	}
	if response.StatusCode != responsePkg.Ok {
		return c.Logger().ErrorWith("点播通道实时音视频失败",
			&gbErrors.SipResponseError{StatusCode: response.StatusCode, ReasonPhrase: response.ReasonPhrase})
	}

	// 根据SIP响应创建通道实时流
	stream, err := c.createStream(response, channel.ChannelId, callIp, callPort, callDomain, transport)
	if err != nil {
		return err
	}

	defer func() {
		// 通道流已经建立，出现错误需要通过BYE请求关闭流
		if err != nil {
			c.doCloseStream(stream)
		}
	}()

	// 解析响应的SDP信息并设置通道实时点播流
	mediaInfo, err := c.parseMessageSDP(&response.Message)
	if err != nil {
		return err
	}

	if err := player.Setup(mediaInfo.RtpMap, mediaInfo.RemoteIP, mediaInfo.RemotePort, mediaInfo.SSRC, true); err != nil {
		return err
	}
	if res := c.dbManager.Table(modelPkg.StreamTableName).Create(stream); res.Error != nil {
		return c.Logger().ErrorWith("向数据库中插入流信息失败", res.Error)
	}
	c.Logger().Info("点播通道实时音视频成功")
	lock.LockDo(&c.streamLock, func() {
		c.stream = stream
		c.streamActive = true
	})
	return nil
}

func (c *Channel) checkStorage() error {
	enableRecord := func() error {
		// 开启媒体通道的音视频录制
		if err := c.mediaChannel.StartRecord(); err != nil {
			return err
		}
		c.recordEnabled = true
		return nil
	}

	setupStorage := func(channel *modelPkg.Channel, storageConfig *modelPkg.StorageConfig) error {
		// 配置媒体通道的存储
		if err := c.mediaChannel.SetupStorage(time.Duration(storageConfig.Cover) * time.Minute); err != nil {
			return err
		}
		c.storageSetup = true
		if channel.EnableRecord {
			return enableRecord()
		}
		return nil
	}

	if channel := c.channel; channel.StorageConfig != "" {
		if c.storageConfig == nil {
			// 存储通道已配置但未加载，加载存储通道
			storageConfig, err := c.storageConfigManager.GetStorageConfig(channel.StorageConfig)
			if err != nil {
				return err
			}
			if storageConfig == nil {
				return c.Logger().ErrorWith("初始化通道失败", &errors.NotFound{Target: "存储配置"},
					log.String("通道", c.name),
					log.String("存储配置", channel.StorageConfig),
				)
			}
			c.storageConfig = storageConfig
			return setupStorage(channel, storageConfig)
		} else {
			if !c.storageSetup {
				return setupStorage(channel, c.storageConfig)
			} else if channel.EnableRecord && !c.recordEnabled {
				return enableRecord()
			}
		}
	}
	return nil
}

func (c *Channel) checkPlayer() error {
	if channel := c.channel; channel.EnableRecord {
		if player := c.mediaChannel.GetRtpPlayer(); player == nil {
			return c.play(channel, true)
		}
	}
	return nil
}

func (c *Channel) check() error {
	if err := c.checkStorage(); err != nil {
		return err
	}
	if err := c.checkPlayer(); err != nil {
		return err
	}
	return nil
}

func (c *Channel) processPlayback(request *sip.Request, mediaInfo *media.MediaInfo) *sip.Response {
	return lock.RLockGet(c.runner, func() *sip.Response {
		if !c.runner.Running() {
			return c.createResponse(request, responsePkg.TemporarilyUnavailable, "")
		}

		// 从SDP的rtpmap中选择合适媒体类型，并生成响应SDP的rtpmap
		level := math.MaxInt
		var rtpMap *sdp.RtpMap
		for typ, name := range mediaInfo.RtpMap {
			if mediaType := media.ParseMediaType(name); mediaType != nil {
				if mediaType.Level < level {
					level = mediaType.Level
					rtpMap = sdp.NewRtpMap(int(typ), name, 90000)
				}
			}
		}
		if rtpMap == nil {
			c.Logger().Error("没有可支持的媒体类型")
			return c.createResponse(request, responsePkg.UnsupportedMediaType, "")
		}

		// 创建SIP对话
		dialog, err := newPlaybackDialog(c, request)
		if err != nil {
			c.Logger().ErrorWith("创建历史音视频回放对话失败", err)
			return c.createResponse(request, responsePkg.BadRequest, "")
		}

		if _, exist := c.playbackDialogs.Load(dialog.DialogId); exist {
			c.Logger().Error("对话已存在，不支持SIP re-INVITE请求", log.String("dialogId", dialog.DialogId))
			return c.createResponse(request, responsePkg.BadRequest, dialog.ToTag)
		}

		// 创建历史音视频RTP推流服务
		pusher, err := c.mediaChannel.OpenHistoryRtpPusher(mediaInfo.RemoteIP, mediaInfo.RemotePort, mediaInfo.Transport,
			mediaInfo.Start.UnixMilli(), mediaInfo.End.UnixMilli(), mediaInfo.SSRC, nil,
			func(pusher *mediaChannel.HistoryRtpPusher, channel *mediaChannel.Channel) {
				dialog.byeOnce.Do(func() {
					c.sendRequest("bye", dialog.createByeRequest())
				})
				c.playbackDialogs.Delete(dialog.DialogId)
			})
		if err != nil {
			return c.createResponse(request, responsePkg.ServerInternalError, dialog.ToTag)
		}
		dialog.Pusher = pusher

		if _, exist := c.playbackDialogs.LoadOrStore(dialog.DialogId, dialog); exist {
			c.mediaChannel.CloseHistoryRtpPusher(pusher.ID())
			c.Logger().Error("对话已存在，不支持SIP re-INVITE请求", log.String("dialogId", dialog.DialogId))
			return c.createResponse(request, responsePkg.BadRequest, dialog.ToTag)
		}

		mediaLocalIp := pusher.LocalIp()
		mediaLocalPort := pusher.LocalPort()

		// 构建SIP INVITE响应
		proxyConfig := c.ProxyConfig()
		sdpSession := c.createResponseSDP("Playback", mediaInfo.Start, mediaInfo.End, mediaLocalIp, mediaLocalPort,
			mediaInfo.Protocol, c.formatSSRC(mediaInfo.SSRC), rtpMap)
		sdpContent := sdpSession.AppendTo(nil)
		response := c.createResponse(request, responsePkg.Ok, dialog.ToTag)
		response.Contact = new(sip.Contact)
		response.Contact.Address.URI = sip.CreateSipURI(proxyConfig.Id, proxyConfig.SipIp, proxyConfig.SipPort, "")
		response.ContentType = "application/sdp"
		response.Content = uns.BytesToString(sdpContent)
		return response
	})
}

func (c *Channel) ProcessProxyInvite(request *sip.Request) *sip.Response {
	mediaInfo, err := c.parseMessageSDP(&request.Message)
	if err != nil {
		return c.createResponse(request, responsePkg.BadRequest, "")
	}

	switch strings.ToLower(mediaInfo.Name) {
	case "play":
		c.Logger().Error("暂不支持客户端点播实时音视频")
		return c.createResponse(request, responsePkg.UnsupportedMediaType, "")
	case "playback":
		return c.processPlayback(request, mediaInfo)
	default:
		return c.createResponse(request, responsePkg.UnsupportedMediaType, "")
	}
}

func (c *Channel) ProcessProxyAck(request *sip.Request) {
	dialogId := sip.CreateDialogId(request.CallId, request.From.Tag, request.To.Tag)
	if dialog, exist := c.playbackDialogs.Load(dialogId); exist {
		dialog.Pusher.StartPush()
	}
}

func (c *Channel) ProcessProxyBye(request *sip.Request) *sip.Response {
	dialogId := sip.CreateDialogId(request.CallId, request.From.Tag, request.To.Tag)
	if dialog, exist := c.playbackDialogs.Load(dialogId); exist {
		dialog.byeOnce.Do(func() {})
		dialog.Pusher.Close(nil)
		return c.createResponse(request, responsePkg.Ok, dialog.ToTag)
	}
	return c.createResponse(request, responsePkg.CallOrTransactionDoesNotExist, "")
}

func (c *Channel) ProcessProxyInfo(request *sip.Request) *sip.Response {
	dialogId := sip.CreateDialogId(request.CallId, request.From.Tag, request.To.Tag)
	if dialog, exist := c.playbackDialogs.Load(dialogId); exist {
		rtsp, err := c.parseMessageMANSRTSP(&request.Message)
		if err != nil {
			c.Logger().ErrorWith("解析MANSRTSP失败", err)
			return c.createResponse(request, responsePkg.UnsupportedMediaType, dialog.ToTag)
		}
		switch rtsp.Method {
		case "PLAY":
			//if rtsp.Scale != 0 {
			//	if rtsp.Scale > 8 || rtsp.Scale < 0.5 {
			//		c.Logger().ErrorMsg("播放倍速超过了限制")
			//		return c.createResponse(request, responsePkg.UnsupportedMediaType, dialog.ToTag)
			//	}
			//}
			if rtsp.Scale != 0 {
				dialog.Pusher.SetScale(rtsp.Scale)
			}
			if rtsp.Range != nil && !rtsp.Range.Now {
				dialog.Pusher.Seek(int64(rtsp.Range.NPT * 1000))
			}
			dialog.Pusher.Resume()
		case "PAUSE":
			dialog.Pusher.Pause()
		case "TEARDOWN":
			go func() {
				dialog.byeOnce.Do(func() {
					c.sendRequest("bye", dialog.createByeRequest())
				})
				dialog.Pusher.Close(nil)
			}()
		default:
			return c.createResponse(request, responsePkg.UnsupportedMediaType, dialog.ToTag)
		}
		return c.createResponse(request, responsePkg.Ok, dialog.ToTag)
	}
	return c.createResponse(request, responsePkg.CallOrTransactionDoesNotExist, "")
}

func (c *Channel) ProcessBye(request *sip.Request) *sip.Response {
	if stream := lock.LockGet(&c.streamLock, func() *modelPkg.Stream {
		if stream := c.stream; stream != nil {
			if stream.Match(request.CallId, request.From.Tag, request.To.Tag) {
				if c.streamActive {
					c.streamActive = false
					return stream
				}
			}
		}
		return nil
	}); stream != nil {
		c.Logger().Info("BYE请求关闭通道实时流成功")
		c.mediaChannel.CloseRtpPlayer()
		c.deleteStream(stream)
		return c.createResponse(request, responsePkg.Ok, "")
	}
	c.Logger().Warn("BYE请求未匹配到对应的实时流")
	return c.createResponse(request, 481, "")
}

func (c *Channel) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	channel := c.channel
	if channel.ChannelId == "" {
		return c.Logger().ErrorWith("初始化通道失败", errors.NewArgumentMissing("channel.channelId"))
	}
	if channel.ChannelIp == "" && channel.Gateway == "" {
		return c.Logger().ErrorWith("初始化通道失败",
			errors.NewArgumentMissingOne("channel.channelIp", "channel.gateway"), log.String("通道", channel.Name))
	}

	switch strings.ToUpper(channel.StreamMode) {
	case "UDP":
		c.mediaTransport = "udp"
		c.mediaProtocol = "RTP/AVP"
	case "TCP-ACTIVE":
		return c.Logger().ErrorWith("初始化通道失败",
			errors.NewInvalidArgument("channel.streamMode", errors.New("暂不支持TCP主动模式")))
	case "TCP-PASSIVE":
		c.mediaTransport = "tcp"
		c.mediaProtocol = "TCP/RTP/AVP"
	default:
		return c.Logger().ErrorWith("初始化通道失败",
			errors.NewInvalidArgument("channel.streamMode", fmt.Errorf("错误的流模式(%s)", channel.StreamMode)))
	}

	// 创建媒体通道
	mc, err := c.mcManager.Create(c.name)
	if err != nil {
		return err
	}
	c.mediaChannel = mc
	return nil
}

func (c *Channel) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	defer func() {
		playbackWaiter := sync.WaitGroup{}
		c.playbackDialogs.Range(func(id string, dialog *PlaybackDialog) bool {
			playbackWaiter.Add(1)
			go func() {
				dialog.byeOnce.Do(func() {
					c.sendRequest("bye", dialog.createByeRequest())
				})
				dialog.Pusher.Close(nil)
				playbackWaiter.Done()
			}()
			return true
		})
		playbackWaiter.Wait()
		c.closeStream()
		<-c.mcManager.Delete(c.name)
	}()

	c.check()
	checkTicker := time.NewTicker(time.Second * 5)
	defer checkTicker.Stop()
	for {
		select {
		case <-checkTicker.C:
			c.check()
		case <-interrupter:
			return nil
		}
	}
}
