package channel

import (
	"errors"
	"fmt"
)

var (
	AllocRTPServerFailed        = errors.New("get rtp server failed")
	NetworkAddressNotFoundError = errors.New("network address not found")
	StreamCreateError           = errors.New("stream create error")
	ChannelClosedError          = errors.New("通道已经关闭或正在关闭")
	StorageChannelNotSetupError = errors.New("存储通道未设置")
)

type HistoryRTPPusherNotFoundError struct {
	ChannelID string
	PusherID  uint64
}

func (e *HistoryRTPPusherNotFoundError) Error() string {
	return fmt.Sprintf("通道(%s)历史音视频RTP推流(%d)不存在", e.ChannelID, e.PusherID)
}

type RTPPlayerNotOpenedError struct {
	ChannelID string
}

func (e *RTPPlayerNotOpenedError) Error() string {
	return fmt.Sprintf("通道(%s)RTP拉流不存在", e.ChannelID)
}
