package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	mediaChannel "gitee.com/sy_183/cvds-mas/media/channel"
	"github.com/gin-gonic/gin"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	MediaChannelModule     = Module + ".media-channel"
	MediaChannelModuleName = "媒体通道管理器API服务"
)

type MediaChannel struct {
	manager *mediaChannel.Manager
	client  http.Client
	log.AtomicLogger
}

func newMediaChannel() *MediaChannel {
	c := &MediaChannel{manager: mediaChannel.GetManager(), client: http.Client{Timeout: time.Second}}
	config.InitModuleLogger(c, MediaChannelModule, MediaChannelModuleName)
	config.RegisterLoggerConfigReloadedCallback(c, MediaChannelModule, MediaChannelModuleName)
	return c
}

func (c *MediaChannel) hookURL(rawUrl string) (*url.URL, error) {
	if rawUrl != "" {
		var err error
		hookURL, err := url.Parse(rawUrl)
		if err != nil {
			return nil, fmt.Errorf("无效的 closedCallback URL(%s), %s", rawUrl, err.Error())
		}
		if hookURL.Scheme != "http" && hookURL.Scheme != "https" {
			return nil, fmt.Errorf("参数解析错误：无效的 closedCallback URL(%s), Scheme 必须为 http 或 https", rawUrl)
		}
	}
	return nil, nil
}

func (c *MediaChannel) notify(url string, obj any) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "CVDSMAS-Channel-Hook")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return nil
}

func (c *MediaChannel) Create(ctx *gin.Context) {
	model := struct {
		ID           string         `json:"id"`
		StorageCover uint           `json:"storageCover"`
		StorageType  string         `json:"storageType"`
		Fields       map[string]any `json:"fields"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ID == "" {
		responseErrorMsg(ctx, "缺少必须的参数(id)", true)
		return
	}

	var options []mediaChannel.Option
	if model.Fields != nil {
		options = append(options, mediaChannel.WithFields(model.Fields))
	}
	if model.StorageType != "" {
		options = append(options, mediaChannel.WithStorageType(model.StorageType))
	}

	ch, err := c.manager.Create(model.ID, options...)
	if err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, ch.Info())
}

func (c *MediaChannel) Get(ctx *gin.Context) {
	id := ctx.Query("id")
	if id == "" {
		responseErrorMsg(ctx, "缺少必须的参数(id)", true)
		return
	}

	ch := c.manager.Get(id)
	if ch == nil {
		responseErrorMsg(ctx, errors.NewNotFound(c.manager.ChannelDisplayName(id)).Error(), true)
		return
	}

	responseSuccess(ctx, ch.Info())
}

func (c *MediaChannel) List(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持列出通道的功能", true)
}

func (c *MediaChannel) Modify(ctx *gin.Context) {
	model := struct {
		ID     string         `json:"id"`
		Fields map[string]any `json:"fields"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ID == "" {
		responseErrorMsg(ctx, "缺少必须的参数(id)", true)
		return
	}

	ch := c.manager.Get(model.ID)
	if ch == nil {
		responseErrorMsg(ctx, errors.NewNotFound(c.manager.ChannelDisplayName(model.ID)).Error(), false)
		return
	}

	for name, value := range model.Fields {
		ch.SetField(name, value)
	}
	responseSuccess(ctx, gin.H{
		"channel": ch.Info(),
	})
}

func (c *MediaChannel) Delete(ctx *gin.Context) {
	id := ctx.Query("id")
	if id == "" {
		responseErrorMsg(ctx, "缺少必须的参数(id)", true)
		return
	}

	if waiter := c.manager.Delete(id); waiter != nil {
		<-waiter
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) OpenRtpPlayer(ctx *gin.Context) {
	model := struct {
		ChannelId  string        `json:"channelId"`
		Transport  string        `json:"transport"`
		Timeout    time.Duration `json:"timeout"`
		ClosedHook string        `json:"closedHook"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	closedHookURL, err := c.hookURL(model.ClosedHook)
	if err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	player, err := c.manager.OpenRtpPlayer(model.ChannelId, model.Transport, model.Timeout, func(player *mediaChannel.RtpPlayer, channel *mediaChannel.Channel) {
		if closedHookURL != nil {
			c.notify(closedHookURL.String(), gin.H{
				"channelId": channel.ID(),
			})
		}
	})
	if err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, player.Info())
}

func (c *MediaChannel) SetupRtpPlayer(ctx *gin.Context) {
	model := struct {
		ChannelId  string           `json:"channelId"`
		RTPMap     map[uint8]string `json:"rtpMap"`
		RemoteIP   net.IP           `json:"remoteIp"`
		RemotePort int              `json:"remotePort"`
		SSRC       *uint32          `json:"ssrc"`
		Once       bool             `json:"once"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.RemotePort < 0 || model.RemotePort > math.MaxInt16 {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：无效的 RemotePort(%d)", model.RemotePort), true)
		ctx.Abort()
	}

	if model.ChannelId == "" || model.RemoteIP == nil || model.RemotePort == 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,mediaType,remoteIp,remotePort)", true)
		return
	}

	ssrc := int64(-1)
	if model.SSRC != nil {
		ssrc = int64(*model.SSRC)
	}

	if err := c.manager.SetupRtpPlayer(model.ChannelId, model.RTPMap, model.RemoteIP, model.RemotePort, ssrc, model.Once); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) GetRtpPlayer(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	player, err := c.manager.GetRtpPlayer(channelId)
	if err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	if player == nil {
		err = &mediaChannel.RTPPlayerNotOpenedError{ChannelID: channelId}
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, player.Info())
}

func (c *MediaChannel) CloseRtpPlayer(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	if waiter, err := c.manager.CloseRtpPlayer(channelId); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	} else if waiter != nil {
		<-waiter
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) OpenRtspPlayer(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持RTSP功能", true)
}

func (c *MediaChannel) CloseRtspPlayer(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持RTSP功能", true)
}

func (c *MediaChannel) SetupStorage(ctx *gin.Context) {
	model := struct {
		ChannelId string `json:"channelId"`
		Cover     int64  `json:"cover"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if err := c.manager.SetupStorage(model.ChannelId, time.Duration(model.Cover)*time.Minute); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) CloseStorage(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	if waiter, err := c.manager.StopStorage(channelId); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	} else if waiter != nil {
		<-waiter
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) StartRecord(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	if err := c.manager.StartRecord(channelId); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) StopRecord(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	if err := c.manager.StopRecord(channelId); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) OpenRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId  string `json:"channelId"`
		TargetIp   net.IP `json:"targetIp"`
		TargetPort int    `json:"targetPort"`
		Transport  string `json:"transport"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.TargetIp == nil || model.TargetPort == 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,targetIp,targetPort)", true)
		return
	}

	responseErrorMsg(ctx, "此版本暂不支持RTP推流功能", true)
}

func (c *MediaChannel) StartRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
	}{PusherId: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	responseErrorMsg(ctx, "此版本暂不支持RTP推流功能", true)
}

func (c *MediaChannel) GetRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
	}{PusherId: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	responseErrorMsg(ctx, "此版本暂不支持RTP推流功能", true)
}

func (c *MediaChannel) ListRtpPusher(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持列出RTP推流列表", true)
}

func (c *MediaChannel) CloseRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
	}{PusherId: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	responseErrorMsg(ctx, "此版本暂不支持RTP推流功能", true)
}

func (c *MediaChannel) OpenHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId  string  `json:"channelId"`
		TargetIp   net.IP  `json:"targetIp"`
		TargetPort int     `json:"targetPort"`
		Transport  string  `json:"transport"`
		StartTime  int64   `json:"startTime"`
		EndTime    int64   `json:"endTime"`
		SSRC       *uint32 `json:"ssrc"`
		EOFHook    string  `json:"eofHook"`
		ClosedHook string  `json:"closedHook"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	eofHookURL, err := c.hookURL(model.EOFHook)
	if err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	closedHookURL, err := c.hookURL(model.ClosedHook)
	if err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.TargetIp == nil || model.TargetPort == 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,targetIp,targetPort)", true)
		return
	}

	ssrc := int64(-1)
	if model.SSRC != nil {
		ssrc = int64(*model.SSRC)
	}

	pusher, err := c.manager.OpenHistoryRtpPusher(model.ChannelId, model.TargetIp, model.TargetPort, model.Transport, model.StartTime, model.EndTime, ssrc,
		func(pusher *mediaChannel.HistoryRtpPusher, channel *mediaChannel.Channel) {
			if eofHookURL != nil {
				c.notify(eofHookURL.String(), gin.H{
					"channelId": channel.ID(),
					"pusherId":  pusher.ID(),
				})
			}
		}, func(pusher *mediaChannel.HistoryRtpPusher, channel *mediaChannel.Channel) {
			if closedHookURL != nil {
				c.notify(closedHookURL.String(), gin.H{
					"channelId": channel.ID(),
					"pusherId":  pusher.ID(),
				})
			}
		})
	if err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, pusher.Info())
}

func (c *MediaChannel) StartHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
	}{PusherId: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if err := c.manager.StartHistoryRtpPusher(model.ChannelId, uint64(model.PusherId)); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) GetHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
	}{PusherId: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	pusher, err := c.manager.GetHistoryRtpPusher(model.ChannelId, uint64(model.PusherId))
	if err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, pusher.Info())
}

func (c *MediaChannel) ListHistoryRtpPusher(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持列出历史音视频RTP推流列表", true)
}

func (c *MediaChannel) PauseHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
	}{PusherId: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if err := c.manager.PauseHistoryRtpPusher(model.ChannelId, uint64(model.PusherId)); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) ResumeHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
	}{PusherId: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if err := c.manager.ResumeHistoryRtpPusher(model.ChannelId, uint64(model.PusherId)); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) SeekHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
		Time      *int64 `form:"time"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 || model.Time == nil {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId,time)", true)
		return
	}

	if err := c.manager.SeekHistoryRtpPusher(model.ChannelId, uint64(model.PusherId), *model.Time); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) SetHistoryRtpPusherScale(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  int64   `form:"pusherId"`
		Scale     float64 `form:"scale"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 || model.Scale == 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if err := c.manager.SetHistoryRtpPusherScale(model.ChannelId, uint64(model.PusherId), model.Scale); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) CloseHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string `form:"channelId"`
		PusherId  int64  `form:"pusherId"`
	}{PusherId: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelId == "" || model.PusherId < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if waiter, err := c.manager.CloseHistoryRtpPusher(model.ChannelId, uint64(model.PusherId)); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	} else if waiter != nil {
		<-waiter
	}
	responseSuccess(ctx, nil)
}

var GetMediaChannel = component.NewPointer(newMediaChannel).Get
