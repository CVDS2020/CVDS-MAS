package ps

import (
	"gitee.com/sy_183/cvds-mas/media/h264"
	rtpFramePkg "gitee.com/sy_183/rtp/frame"
)

type RtpConverter struct {
	frameParser *FrameParser

	h264NaluParser  *h264.AnnexBInPrefixNALUParser
	h264RtpPackager *h264.RtpPackager
}

func (c *RtpConverter) Convert(rtpFrame rtpFramePkg.Frame) {

}
