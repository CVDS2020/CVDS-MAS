package endpoint

import (
	"encoding/json"
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/uns"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/media"
	"io"
	"net"
	"net/http"
	"strconv"
)

const (
	Module     = "zongheng.endpoint"
	ModuleName = "纵横接口"
)

type Endpoint struct {
	addr   string
	client *http.Client
	log.AtomicLogger
}

func NewEndpoint() *Endpoint {
	cfg := config.ZongHengEndpointConfig()
	e := &Endpoint{
		addr:   cfg.Addr,
		client: &http.Client{Timeout: cfg.Timeout},
	}
	if e.addr == "" {
		e.addr = "127.0.0.1:18080"
	}
	config.InitModuleLogger(e, Module, ModuleName)
	config.RegisterLoggerConfigReloadedCallback(e, Module, ModuleName)
	return e
}

func (e *Endpoint) StartStream(channelId string, deviceId string, transport string, localIP net.IP, localPort int) (mediaInfo *media.MediaInfo, err error) {
	if ipv4 := localIP.To4(); ipv4 != nil {
		localIP = ipv4
	}

	// generate request url
	url := fmt.Sprintf("http://%s/api/stream/start?channelId=%s&deviceId=%s&ip=%s&port=%d",
		e.addr,
		channelId,
		deviceId,
		localIP.String(),
		localPort,
	)
	// request media stream
	res, err := e.client.Get(url)
	if err != nil {
		return nil, e.Logger().ErrorWith("发送HTTP请求失败", err, log.String("url", url))
	}
	// parse response
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, e.Logger().ErrorWith("读取HTTP响应结果失败", err)
	}
	model := struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			IP   string `json:"ip"`
			Port string `json:"port"`
			SSRC string `json:"ssrc"`
			SDP  string `json:"sdp"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, e.Logger().ErrorWith("解析HTTP响应结果失败", err)
	}
	if model.Code != 0 {
		return nil, e.Logger().ErrorWith("HTTP响应失败", &ResultStatusError{Code: model.Code, Msg: model.Msg})
	}

	if model.Data.SDP != "" {
		// 返回数据中包含SDP信息
		mediaInfo, err = media.NewMediaInfoFromSDP(uns.StringToBytes(model.Data.SDP))
		if err != nil {
			if e, ok := err.(interface{ AddParentArgument(parent string) }); ok {
				e.AddParentArgument("sdp")
			}
			return nil, e.Logger().ErrorWith("解析SDP信息失败", err)
		}
	} else {
		mediaInfo = &media.MediaInfo{
			Transport: transport,
			SSRC:      -1,
		}
		// 没有返回SDP信息，使用
		if model.Data.Port != "" {
			u, err := strconv.ParseUint(model.Data.Port, 10, 16)
			if err != nil {
				return nil, e.Logger().ErrorWith("解析实时流端口失败", errors.NewInvalidArgument("port", err))
			}
			mediaInfo.RemotePort = int(u)
		}
		// parse remote ip
		if model.Data.IP != "" {
			remoteIPAddr, err := net.ResolveIPAddr("ip", model.Data.IP)
			if err != nil {
				return nil, e.Logger().ErrorWith("解析实时流IP失败", errors.NewInvalidArgument("ip", err))
			}
			mediaInfo.RemoteIP = remoteIPAddr.IP
		}
	}

	// 如果SDP中没有SSRC信息，但是返回属性中包含，则使用返回属性中的SSRC
	if mediaInfo.SSRC < 0 {
		if model.Data.SSRC != "" {
			u, err := strconv.ParseUint(model.Data.SSRC, 10, 32)
			if err != nil {
				return nil, e.Logger().ErrorWith("解析实时流SSRC失败", errors.NewInvalidArgument("ssrc", err))
			}
			mediaInfo.SSRC = int64(u)
		}
	}

	e.Logger().Info("点播实时流成功", log.Object("媒体信息", mediaInfo))
	return mediaInfo, nil
}

func (e *Endpoint) StopStream(channelId string, deviceId string) error {
	url := fmt.Sprintf("http://%s/api/stream/stop?channelId=%s&deviceId=%s", e.addr, channelId, deviceId)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return e.Logger().ErrorWith("发送HTTP请求失败", err, log.String("url", url))
	}
	res, err := e.client.Do(req)
	if err != nil {
		return e.Logger().ErrorWith("发送HTTP请求失败", err, log.String("url", url))
	}
	// parse response
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return e.Logger().ErrorWith("读取HTTP响应结果失败", err)
	}
	model := struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}{}
	if err := json.Unmarshal(data, &model); err != nil {
		return e.Logger().ErrorWith("解析HTTP响应结果失败", err)
	}
	if model.Code != 0 {
		return e.Logger().ErrorWith("HTTP响应失败", &ResultStatusError{Code: model.Code, Msg: model.Msg})
	}

	e.Logger().Info("关闭实时流成功", log.String("设备ID", deviceId), log.String("通道ID", channelId))
	return nil
}

var GetEndpoint = component.NewPointer(NewEndpoint).Get
