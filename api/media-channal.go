package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gitee.com/sy_183/common/component"
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
	MediaChannelModule     = "api.media-channel"
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

	ch, err := c.manager.Create(model.ID, model.Fields)
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
		err := &mediaChannel.ChannelNotFoundError{ID: id}
		responseErrorMsg(ctx, err.Error(), true)
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
		err := &mediaChannel.ChannelNotFoundError{ID: model.ID}
		responseErrorMsg(ctx, err.Error(), false)
		return
	}

	var modified = make([]string, 0)
	for name, value := range model.Fields {
		if ch.ModifyField(name, value) {
			modified = append(modified, name)
		}
	}
	responseSuccess(ctx, gin.H{
		"modified": modified,
		"channel":  ch.Info(),
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

func (c *MediaChannel) OpenRTPPlayer(ctx *gin.Context) {
	model := struct {
		ChannelID  string        `json:"channelId"`
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

	if model.ChannelID == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	player, err := c.manager.OpenRTPPlayer(model.ChannelID, model.Transport, model.Timeout, func(player *mediaChannel.RTPPlayer, channel *mediaChannel.Channel) {
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

func (c *MediaChannel) SetupRTPPlayer(ctx *gin.Context) {
	model := struct {
		ChannelID  string           `json:"channelId"`
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

	if model.ChannelID == "" || model.RemoteIP == nil || model.RemotePort == 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,mediaType,remoteIp,remotePort)", true)
		return
	}

	ssrc := int64(-1)
	if model.SSRC != nil {
		ssrc = int64(*model.SSRC)
	}

	if err := c.manager.SetupRTPPlayer(model.ChannelID, model.RTPMap, model.RemoteIP, model.RemotePort, ssrc, model.Once); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) GetRTPPlayer(ctx *gin.Context) {
	channelID := ctx.Query("channelId")
	if channelID == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	player, err := c.manager.GetRTPPlayer(channelID)
	if err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	if player == nil {
		err = &mediaChannel.RTPPlayerNotOpenedError{ChannelID: channelID}
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, player.Info())
}

func (c *MediaChannel) CloseRTPPlayer(ctx *gin.Context) {
	channelID := ctx.Query("channelId")
	if channelID == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	if waiter, err := c.manager.CloseRTPPlayer(channelID); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	} else if waiter != nil {
		<-waiter
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) OpenRTSPPlayer(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持RTSP功能", true)
}

func (c *MediaChannel) CloseRTSPPlayer(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持RTSP功能", true)
}

func (c *MediaChannel) SetupStorage(ctx *gin.Context) {
	model := struct {
		ChannelID string `json:"channelId"`
		Cover     int64  `json:"cover"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if err := c.manager.SetupStorage(model.ChannelID, time.Duration(model.Cover)*time.Minute); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) CloseStorage(ctx *gin.Context) {
	channelID := ctx.Query("channelId")
	if channelID == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	if waiter, err := c.manager.StopStorage(channelID); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	} else if waiter != nil {
		<-waiter
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) StartRecord(ctx *gin.Context) {
	channelID := ctx.Query("channelId")
	if channelID == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	if err := c.manager.StartRecord(channelID); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) StopRecord(ctx *gin.Context) {
	channelID := ctx.Query("channelId")
	if channelID == "" {
		responseErrorMsg(ctx, "缺少必须的参数(channelId)", true)
		return
	}

	if err := c.manager.StopRecord(channelID); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) OpenRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID  string `json:"channelId"`
		TargetIP   net.IP `json:"targetIp"`
		TargetPort int    `json:"targetPort"`
		Transport  string `json:"transport"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.TargetIP == nil || model.TargetPort == 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,targetIp,targetPort)", true)
		return
	}

	responseErrorMsg(ctx, "此版本暂不支持RTP推流功能", true)
}

func (c *MediaChannel) StartRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
	}{PusherID: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	responseErrorMsg(ctx, "此版本暂不支持RTP推流功能", true)
}

func (c *MediaChannel) GetRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
	}{PusherID: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	responseErrorMsg(ctx, "此版本暂不支持RTP推流功能", true)
}

func (c *MediaChannel) ListRTPPusher(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持列出RTP推流列表", true)
}

func (c *MediaChannel) CloseRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
	}{PusherID: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	responseErrorMsg(ctx, "此版本暂不支持RTP推流功能", true)
}

func (c *MediaChannel) OpenHistoryRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID  string  `json:"channelId"`
		TargetIP   net.IP  `json:"targetIp"`
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

	if model.ChannelID == "" || model.TargetIP == nil || model.TargetPort == 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,targetIp,targetPort)", true)
		return
	}

	ssrc := int64(-1)
	if model.SSRC != nil {
		ssrc = int64(*model.SSRC)
	}

	pusher, err := c.manager.OpenHistoryRTPPusher(model.ChannelID, model.TargetIP, model.TargetPort, model.Transport, model.StartTime, model.EndTime, ssrc,
		func(pusher *mediaChannel.HistoryRTPPusher, channel *mediaChannel.Channel) {
			if eofHookURL != nil {
				c.notify(eofHookURL.String(), gin.H{
					"channelId": channel.ID(),
					"pusherId":  pusher.ID(),
				})
			}
		}, func(pusher *mediaChannel.HistoryRTPPusher, channel *mediaChannel.Channel) {
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

func (c *MediaChannel) StartHistoryRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
	}{PusherID: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if err := c.manager.StartHistoryRTPPusher(model.ChannelID, uint64(model.PusherID)); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) GetHistoryRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
	}{PusherID: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	pusher, err := c.manager.GetHistoryRTPPusher(model.ChannelID, uint64(model.PusherID))
	if err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, pusher.Info())
}

func (c *MediaChannel) ListHistoryRTPPusher(ctx *gin.Context) {
	responseErrorMsg(ctx, "此版本暂不支持列出历史音视频RTP推流列表", true)
}

func (c *MediaChannel) PauseHistoryRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
	}{PusherID: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if err := c.manager.PauseHistoryRTPPusher(model.ChannelID, uint64(model.PusherID)); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) ResumeHistoryRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
	}{PusherID: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if err := c.manager.ResumeHistoryRTPPusher(model.ChannelID, uint64(model.PusherID)); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) SeekHistoryRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
		Time      *int64 `form:"time"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 || model.Time == nil {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId,time)", true)
		return
	}

	if err := c.manager.SeekHistoryRTPPusher(model.ChannelID, uint64(model.PusherID), *model.Time); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) SetHistoryRTPPusherScale(ctx *gin.Context) {
	model := struct {
		ChannelID string  `form:"channelId"`
		PusherID  int64   `form:"pusherId"`
		Scale     float64 `form:"scale"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 || model.Scale == 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if err := c.manager.SetHistoryRTPPusherScale(model.ChannelID, uint64(model.PusherID), model.Scale); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) CloseHistoryRTPPusher(ctx *gin.Context) {
	model := struct {
		ChannelID string `form:"channelId"`
		PusherID  int64  `form:"pusherId"`
	}{PusherID: -1}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseErrorMsg(ctx, fmt.Sprintf("参数解析错误：%s", err.Error()), true)
		return
	}

	if model.ChannelID == "" || model.PusherID < 0 {
		responseErrorMsg(ctx, "缺少必须的参数(channelId,pusherId)", true)
		return
	}

	if waiter, err := c.manager.CloseHistoryRTPPusher(model.ChannelID, uint64(model.PusherID)); err != nil {
		responseErrorMsg(ctx, err.Error(), false)
		return
	} else if waiter != nil {
		<-waiter
	}
	responseSuccess(ctx, nil)
}

var GetMediaChannel = component.NewPointer(newMediaChannel).Get
