package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/media"
	mediaChannel "gitee.com/sy_183/cvds-mas/media/channel"
	"github.com/gin-gonic/gin"
	"io"
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
			return nil, err
		}
		if hookURL.Scheme != "http" && hookURL.Scheme != "https" {
			return nil, fmt.Errorf("无效的 url scheme(%s), Scheme 必须为 http 或 https", hookURL.Scheme)
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
		ChannelId   string         `json:"channelId"`
		StorageType string         `json:"storageType"`
		Fields      map[string]any `json:"fields"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	var options []mediaChannel.Option
	if model.Fields != nil {
		options = append(options, mediaChannel.WithFields(model.Fields))
	}
	if model.StorageType != "" {
		options = append(options, mediaChannel.WithStorageType(model.StorageType))
	}

	ch, err := c.manager.Create(model.ChannelId, options...)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, ch.Info())
}

func (c *MediaChannel) Get(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	ch, err := c.manager.Get(channelId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, ch.Info())
}

func (c *MediaChannel) Modify(ctx *gin.Context) {
	model := struct {
		ChannelId string         `json:"channelId"`
		Fields    map[string]any `json:"fields"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	ch, err := c.manager.Get(model.ChannelId)
	if err != nil {
		responseError(ctx, err, false)
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
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	waiter, err := c.manager.Delete(channelId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	<-waiter
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) OpenRtpPlayer(ctx *gin.Context) {
	model := struct {
		ChannelId  string `json:"channelId"`
		ClosedHook string `json:"closedHook"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	closedHookURL, err := c.hookURL(model.ClosedHook)
	if err != nil {
		responseError(ctx, errors.NewInvalidArgument("closedHook", err), true)
		return
	}

	if model.ChannelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	player, err := c.manager.OpenRtpPlayer(model.ChannelId, func(player *mediaChannel.RtpPlayer, channel *mediaChannel.Channel) {
		if closedHookURL != nil {
			c.notify(closedHookURL.String(), gin.H{
				"channelId": channel.ID(),
			})
		}
	})
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, player.Info())
}

func (c *MediaChannel) GetRtpPlayer(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	player, err := c.manager.GetRtpPlayer(channelId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, player.Info())
}

func (c *MediaChannel) RtpPlayerAddStream(ctx *gin.Context) {
	model := struct {
		ChannelId string `json:"channelId"`
		Transport string `json:"transport"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.Transport == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId", "transport"), true)
		return
	}

	streamPlayer, err := c.manager.RtpPlayerAddStream(model.ChannelId, model.Transport)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, streamPlayer.Info())
}

func (c *MediaChannel) RtpPlayerSetupStream(ctx *gin.Context) {
	model := struct {
		ChannelId  string  `json:"channelId"`
		StreamId   *uint64 `json:"streamId"`
		RemoteIp   net.IP  `json:"remoteIp"`
		RemotePort int     `json:"remotePort"`
		SSRC       int64   `json:"ssrc"`
		StreamInfo *struct {
			MediaType   string `json:"mediaType"`
			PayloadType uint8  `json:"payloadType"`
			ClockRate   int    `json:"clockRate"`
		} `json:"streamInfo"`
		SetupConfig mediaChannel.SetupConfig
	}{SSRC: -1}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.StreamId == nil || model.StreamInfo == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "streamId", "streamInfo"), true)
		return
	}

	if model.StreamInfo.PayloadType >= 128 {
		responseError(ctx, errors.NewInvalidArgument("streamInfo.payloadType",
			fmt.Errorf("无效的RTP负载类型(%d)", model.StreamInfo.PayloadType)), true)
		return
	}

	mediaType := media.ParseMediaType(model.StreamInfo.MediaType)
	if mediaType == nil {
		responseError(ctx, errors.NewInvalidArgument("streamInfo.mediaType",
			fmt.Errorf("无效的媒体类型(%s)", model.StreamInfo.MediaType)), true)
		return
	}
	streamInfo := media.NewRtpStreamInfo(mediaType, nil, model.StreamInfo.PayloadType, model.StreamInfo.ClockRate)

	if err := c.manager.RtpPlayerSetupStream(model.ChannelId, *model.StreamId, model.RemoteIp, model.RemotePort, model.SSRC, streamInfo, model.SetupConfig); err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) RtpPlayerRemoveStream(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		StreamId  *uint64 `form:"streamId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.StreamId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "streamId"), true)
		return
	}

	if err := c.manager.RtpPlayerRemoveStream(model.ChannelId, *model.StreamId); err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) CloseRtpPlayer(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	waiter, err := c.manager.CloseRtpPlayer(channelId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	<-waiter
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) SetupStorage(ctx *gin.Context) {
	model := struct {
		ChannelId  string `json:"channelId"`
		Cover      int64  `json:"cover"`
		BufferSize uint   `json:"bufferSize"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	if err := c.manager.SetupStorage(model.ChannelId, time.Duration(model.Cover)*time.Minute, model.BufferSize); err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) StopStorage(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	waiter, err := c.manager.StopStorage(channelId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	<-waiter
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) StartRecord(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	if err := c.manager.StartRecord(channelId); err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) StopRecord(ctx *gin.Context) {
	channelId := ctx.Query("channelId")
	if channelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	if err := c.manager.StopRecord(channelId); err != nil {
		responseError(ctx, err, false)
		return
	}
	responseSuccess(ctx, nil)
}

func (c *MediaChannel) OpenRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId  string `json:"channelId"`
		MaxCached  int    `json:"maxCached"`
		ClosedHook string `json:"closedHook"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	closedHookURL, err := c.hookURL(model.ClosedHook)
	if err != nil {
		responseError(ctx, errors.NewInvalidArgument("closedHook", err), true)
		return
	}

	pusher, err := c.manager.OpenRtpPusher(model.ChannelId, model.MaxCached, func(pusher *mediaChannel.RtpPusher, channel *mediaChannel.Channel) {
		if closedHookURL != nil {
			c.notify(closedHookURL.String(), gin.H{
				"channelId": channel.ID(),
				"pusherId":  pusher.Id(),
			})
		}
	})
	if err != nil {
		responseError(ctx, err, true)
		return
	}

	responseSuccess(ctx, pusher.Info())
}

func (c *MediaChannel) GetRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	pusher, err := c.manager.GetRtpPusher(model.ChannelId, *model.PusherId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, pusher.Info())
}

func (c *MediaChannel) RtpPusherAddStream(ctx *gin.Context) {
	model := struct {
		ChannelId  string  `json:"channelId"`
		PusherId   *uint64 `json:"pusherId"`
		Transport  string  `json:"transport"`
		RemoteIp   net.IP  `json:"remoteIp"`
		RemotePort int     `json:"remotePort"`
		SSRC       int64   `json:"ssrc"`
		StreamInfo *struct {
			MediaType   string `json:"mediaType"`
			PayloadType uint8  `json:"payloadType"`
			ClockRate   int    `json:"clockRate"`
		} `json:"streamInfo"`
	}{SSRC: -1}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil || model.Transport == "" || model.StreamInfo == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId", "transport", "streamInfo"), true)
		return
	}

	if model.StreamInfo.PayloadType >= 128 {
		responseError(ctx, errors.NewInvalidArgument("streamInfo.payloadType",
			fmt.Errorf("无效的RTP负载类型(%d)", model.StreamInfo.PayloadType)), true)
		return
	}

	mediaType := media.ParseMediaType(model.StreamInfo.MediaType)
	if mediaType == nil {
		responseError(ctx, errors.NewInvalidArgument("streamInfo.mediaType",
			fmt.Errorf("无效的媒体类型(%s)", model.StreamInfo.MediaType)), true)
		return
	}
	streamInfo := media.NewRtpStreamInfo(mediaType, nil, model.StreamInfo.PayloadType, model.StreamInfo.ClockRate)

	streamPusher, err := c.manager.RtpPusherAddStream(model.ChannelId, *model.PusherId, model.Transport, model.RemoteIp, model.RemotePort, model.SSRC, streamInfo)
	if err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, streamPusher.Info())
}

func (c *MediaChannel) RtpPusherStartPush(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	if err := c.manager.RtpPusherStartPush(model.ChannelId, *model.PusherId); err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) RtpPusherPause(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	if err := c.manager.RtpPusherPause(model.ChannelId, *model.PusherId); err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) RtpPusherRemoveStream(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
		StreamId  *uint64 `form:"streamId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil || model.StreamId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId", "streamId"), true)
		return
	}

	if err := c.manager.RtpPusherRemoveStream(model.ChannelId, *model.PusherId, *model.StreamId); err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) CloseRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	waiter, err := c.manager.CloseRtpPusher(model.ChannelId, *model.PusherId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	<-waiter

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) OpenHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId  string `json:"channelId"`
		StartTime  int64  `json:"startTime"`
		EndTime    int64  `json:"endTime"`
		EOFHook    string `json:"eofHook"`
		ClosedHook string `json:"closedHook"`
	}{}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" {
		responseError(ctx, errors.NewArgumentMissing("channelId"), true)
		return
	}

	eofHookURL, err := c.hookURL(model.EOFHook)
	if err != nil {
		responseError(ctx, errors.NewInvalidArgument("eofHook", err), true)
		return
	}

	closedHookURL, err := c.hookURL(model.ClosedHook)
	if err != nil {
		responseError(ctx, errors.NewInvalidArgument("closedHook", err), true)
		return
	}

	pusher, err := c.manager.OpenHistoryRtpPusher(model.ChannelId, model.StartTime, model.EndTime, func(pusher *mediaChannel.HistoryRtpPusher, channel *mediaChannel.Channel) {
		if eofHookURL != nil {
			c.notify(eofHookURL.String(), gin.H{
				"channelId": channel.ID(),
				"pusherId":  pusher.Id(),
			})
		}
	}, func(pusher *mediaChannel.HistoryRtpPusher, channel *mediaChannel.Channel) {
		if closedHookURL != nil {
			c.notify(closedHookURL.String(), gin.H{
				"channelId": channel.ID(),
				"pusherId":  pusher.Id(),
			})
		}
	})
	if err != nil {
		responseError(ctx, err, true)
		return
	}

	responseSuccess(ctx, pusher.Info())
}

func (c *MediaChannel) GetHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	pusher, err := c.manager.GetHistoryRtpPusher(model.ChannelId, *model.PusherId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, pusher.Info())
}

func (c *MediaChannel) HistoryRtpPusherAddStream(ctx *gin.Context) {
	model := struct {
		ChannelId  string  `json:"channelId"`
		PusherId   *uint64 `json:"pusherId"`
		Transport  string  `json:"transport"`
		RemoteIp   net.IP  `json:"remoteIp"`
		RemotePort int     `json:"remotePort"`
		SSRC       int64   `json:"ssrc"`
		StreamInfo *struct {
			MediaType   string `json:"mediaType"`
			PayloadType uint8  `json:"payloadType"`
			ClockRate   int    `json:"clockRate"`
		} `json:"streamInfo"`
	}{SSRC: -1}

	if err := ctx.ShouldBindJSON(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil || model.Transport == "" || model.StreamInfo == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId", "transport", "streamInfo"), true)
		return
	}

	if model.StreamInfo.PayloadType >= 128 {
		responseError(ctx, errors.NewInvalidArgument("streamInfo.payloadType",
			fmt.Errorf("无效的RTP负载类型(%d)", model.StreamInfo.PayloadType)), true)
		return
	}

	mediaType := media.ParseMediaType(model.StreamInfo.MediaType)
	if mediaType == nil {
		responseError(ctx, errors.NewInvalidArgument("streamInfo.mediaType",
			fmt.Errorf("无效的媒体类型(%s)", model.StreamInfo.MediaType)), true)
		return
	}
	streamInfo := media.NewRtpStreamInfo(mediaType, nil, model.StreamInfo.PayloadType, model.StreamInfo.ClockRate)

	streamPusher, err := c.manager.HistoryRtpPusherAddStream(model.ChannelId, *model.PusherId, model.Transport, model.RemoteIp, model.RemotePort, model.SSRC, streamInfo)
	if err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, streamPusher.Info())
}

func (c *MediaChannel) HistoryRtpPusherStartPush(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	if err := c.manager.HistoryRtpPusherStartPush(model.ChannelId, *model.PusherId); err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) HistoryRtpPusherPause(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	if err := c.manager.HistoryRtpPusherPause(model.ChannelId, *model.PusherId); err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) HistoryRtpPusherSeek(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
		Time      int64   `form:"time"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	if err := c.manager.HistoryRtpPusherSeek(model.ChannelId, *model.PusherId, model.Time); err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) HistoryRtpPusherSetScale(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
		Scale     float64 `form:"scale"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil || model.Scale == 0 {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	if err := c.manager.HistoryRtpPusherSetScale(model.ChannelId, *model.PusherId, model.Scale); err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) HistoryRtpPusherRemoveStream(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
		StreamId  *uint64 `form:"streamId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil || model.StreamId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId", "streamId"), true)
		return
	}

	if err := c.manager.HistoryRtpPusherRemoveStream(model.ChannelId, *model.PusherId, *model.StreamId); err != nil {
		responseError(ctx, err, false)
		return
	}

	responseSuccess(ctx, nil)
}

func (c *MediaChannel) CloseHistoryRtpPusher(ctx *gin.Context) {
	model := struct {
		ChannelId string  `form:"channelId"`
		PusherId  *uint64 `form:"pusherId"`
	}{}

	if err := ctx.ShouldBindQuery(&model); err != nil {
		responseError(ctx, errors.NewInvalidArgument("", err), true)
		return
	}

	if model.ChannelId == "" || model.PusherId == nil {
		responseError(ctx, errors.NewArgumentMissing("channelId", "pusherId"), true)
		return
	}

	waiter, err := c.manager.CloseHistoryRtpPusher(model.ChannelId, *model.PusherId)
	if err != nil {
		responseError(ctx, err, false)
		return
	}
	<-waiter

	responseSuccess(ctx, nil)
}

var GetMediaChannel = component.NewPointer(newMediaChannel).Get
