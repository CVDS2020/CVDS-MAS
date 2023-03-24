package rtsp

import "net/url"

type Request struct {
	Method     string
	URL        *url.URL
	RequestURI string

	Proto      string
	ProtoMajor int
	ProtoMinor int

	Message
}
