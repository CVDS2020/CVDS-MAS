package rtsp2

import (
	"fmt"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"io"
	"net/textproto"
)

type ResponseI interface {
	Proto() string

	ProtoMajor() int

	ProtoMinor() int

	SetProto(major, minor int)

	StatusCode() int

	SetStatusCode(code int)

	ReasonPhrase() string

	SetReasonPhrase(reasonPhrase string)

	MessageI
}

type Response struct {
	proto      string
	protoMajor int
	protoMinor int

	statusCode   int
	reasonPhrase string

	Message
}

func NewResponse(request *Request) *Response {
	return &Response{
		protoMajor:   1,
		protoMinor:   0,
		statusCode:   200,
		reasonPhrase: "OK",
		Message: Message{
			header: make(textproto.MIMEHeader),
			cSeq:   request.cSeq,
		},
	}
}

func (r *Response) SetStatus(statusCode int, reasonPhrase string) {
	r.statusCode, r.reasonPhrase = statusCode, reasonPhrase
}

func (r *Response) WriteTo(w io.Writer) (n int64, err error) {
	startLine := fmt.Sprintf("RTSP/%d.%d %03d %s\r\n", r.protoMajor, r.protoMinor, r.statusCode, r.reasonPhrase)
	if err = ioUtils.WriteString(w, startLine, &n); err != nil {
		return
	}
	err = ioUtils.WriteTo(&r.Message, w, &n)
	return
}
