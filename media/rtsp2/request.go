package rtsp2

import "net/url"

type RequestI interface {
	Method() string

	SetMethod(method string)

	URL() *url.URL

	Proto() string

	ProtoMajor() int

	ProtoMinor() int

	SetProto(major, minor int)

	MessageI
}

type Request struct {
	method     string
	url        *url.URL
	requestURI string

	proto      string
	protoMajor int
	protoMinor int

	Message
}
