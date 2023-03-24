package rtp

import (
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/option"
	"gitee.com/sy_183/cvds-mas/media/ps"
	"gitee.com/sy_183/rtp/rtp"
)

type Packager interface {
	Package(pes *ps.PES, rtpAppender func(packet rtp.Packet) bool) error

	Complete(rtpAppender func(packet rtp.Packet) bool) error
}

type PackagerProvider func(options ...option.Option[Packager, any]) Packager

var packagerProviderMap container.SyncMap[uint32, PackagerProvider]

func RegisterPackagerProvider(mediaType uint32, provider PackagerProvider) {
	packagerProviderMap.Store(mediaType, provider)
}

func FindPackagerProvider(mediaType uint32) PackagerProvider {
	p, _ := packagerProviderMap.Load(mediaType)
	return p
}
