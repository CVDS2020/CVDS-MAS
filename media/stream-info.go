package media

import (
	"gitee.com/sy_183/common/log"
)

type StreamInfo interface {
	MediaType() *MediaType

	Parent() StreamInfo

	Equal(StreamInfo) bool
}

type BaseStreamInfo struct {
	mediaType *MediaType
	parent    StreamInfo
}

func (i *BaseStreamInfo) Init(mediaType *MediaType, parent StreamInfo) {
	i.mediaType = mediaType
	i.parent = parent
}

func (i *BaseStreamInfo) MediaType() *MediaType {
	return i.mediaType
}

func (i *BaseStreamInfo) Parent() StreamInfo {
	return i.parent
}

func (i *BaseStreamInfo) Equal(other StreamInfo) bool {
	panic("not implement")
}

type RtpStreamInfo struct {
	BaseStreamInfo
	payloadType uint8
	clockRate   int
}

func NewRtpStreamInfo(mediaType *MediaType, parent StreamInfo, payloadType uint8, clockRate int) *RtpStreamInfo {
	i := &RtpStreamInfo{payloadType: payloadType, clockRate: clockRate}
	i.Init(mediaType, parent)
	return i
}

func (i *RtpStreamInfo) PayloadType() uint8 {
	if i.payloadType >= 128 {
		return 128
	}
	return i.payloadType
}

func (i *RtpStreamInfo) ClockRate() int {
	if i.clockRate <= 0 {
		return i.mediaType.ClockRate
	}
	return i.clockRate
}

func (i *RtpStreamInfo) Equal(other StreamInfo) bool {
	if i == nil && other == nil {
		return true
	} else if i == nil || other == nil {
		return false
	}
	o, ok := other.(*RtpStreamInfo)
	if !ok || o == nil {
		return false
	}
	return i.mediaType.ID == o.mediaType.ID && i.PayloadType() == o.PayloadType() && i.ClockRate() == o.ClockRate()
}

func (i *RtpStreamInfo) MarshalLogObject(encoder log.ObjectEncoder) error {
	encoder.AddUint32("媒体类型ID", i.mediaType.ID)
	encoder.AddString("媒体类型名称", i.mediaType.Name)
	encoder.AddString("媒体类型", i.mediaType.Type)
	if i.payloadType < 128 {
		encoder.AddUint8("流负载类型", i.payloadType)
	}
	encoder.AddInt("时钟频率", i.ClockRate())
	return nil
}
