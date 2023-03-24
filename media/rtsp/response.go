package rtsp

import (
	"fmt"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"io"
)

type Response struct {
	Proto      string
	ProtoMajor int
	ProtoMinor int

	StatusCode   int
	ReasonPhrase string

	Message
}

func (r *Response) WriteTo(w io.Writer) (n int64, err error) {
	startLine := fmt.Sprintf("RTSP/%d.%d %03d %s\r\n", r.ProtoMajor, r.ProtoMinor, r.StatusCode, r.ReasonPhrase)
	if err = ioUtils.WriteString(w, startLine, &n); err != nil {
		return
	}
	err = ioUtils.WriteTo(&r.Message, w, &n)
	return
}
