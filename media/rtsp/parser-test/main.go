package main

import (
	"fmt"
	"gitee.com/sy_183/common/uns"
	"gitee.com/sy_183/cvds-mas/media/rtsp"
	"strings"
)

func parse(parser *rtsp.Parser, messages ...string) {
	for _, msg := range messages {
		remain := uns.StringToBytes(msg)
		for len(remain) > 0 {
			ok, err := parser.ParseP(remain, &remain)
			if err != nil {
				fmt.Println(err)
			} else if ok {
				switch parser.PacketType() {
				case rtsp.PacketTypeRequest:
					fmt.Println(parser.Request())
				case rtsp.PacketTypeResponse:
					fmt.Println(parser.Response())
				case rtsp.PacketTypeRtp:
					fmt.Println(parser.RtpPacket())
				}
			}
		}
	}
}

func main() {

	parser := rtsp.NewParser(0, 0, 0)

	//	message := strings.ReplaceAll(`OPTIONS rtsp://10.3.8.202:554 RTSP/1.0
	//CSeq: 2
	//User-Agent: LibVLC/2.2.8 (LIVE555 Streaming Media v2016.02.22)
	//
	//`, "\n", "\r\n")
	//
	//	parse(parser, message)
	//
	//	message = strings.ReplaceAll(`DESCRIBE rtsp://10.3.8.202:554 RTSP/1.0
	//CSeq: 3
	//User-Agent: LibVLC/2.2.8 (LIVE555 Streaming Media v2016.02.22)
	//Accept: application/sdp
	//
	//`, "\n", "\r\n")
	//
	//	parse(parser, message)

	messages := []string{strings.ReplaceAll(`RTSP/1.0 200 OK
CSeq: ss
Content-Type: applicat`, "\n", "\r\n"), strings.ReplaceAll(`ion/sdp
Content-Base: rtsp://10.3.8.202:554/
Content-Length: 551

`, "\n", "\r\n"), strings.ReplaceAll(`v=0
o=- 1517245007527432 1517245007`, "\n", "\r\n"), strings.ReplaceAll(`527432 IN IP4 10.3.8.202
s=Media Presentation
e=NONE
b=AS:5050
t=0 0
a=control:rt`, "\n", "\r\n"), strings.ReplaceAll(`sp://10.3.8.202:554/
m=video 0 RTP/AVP 96
c=IN IP4 0.0.0.0
b=AS:5000
a=rec`, "\n", "\r\n"), strings.ReplaceAll(`vonly
a=x-dimensions:2048,1536
a=control:rtsp://10.3.8.202:554/trackID=1
a=rtpmap:96 H264/90000
a=fmtp:96 profile-level-id=420029; packetization-mode=1; sprop-parameter-sets=Z00AMp2oCAAwabgICAoAAAMAAgAAAwBlCA==,aO48gA==
a=Media_header:MEDIAINFO=494D4B48010200000400000100000000000000000000000000000000000000000000000000000000;
a=appversion:1.0
`, "\n", "\r\n")}

	parse(parser, messages...)
}
