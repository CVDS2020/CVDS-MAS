package rtsp

import (
	"fmt"
	"gitee.com/sy_183/common/pool"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"io"
	"net/textproto"
)

type Message struct {
	Header        textproto.MIMEHeader
	CSeq          int
	ContentType   string
	ContentLength int64
	Session       string
	Body          [][]byte

	pool.AtomicRef
}

func (m *Message) WriteTo(w io.Writer) (n int64, err error) {
	defer func() { err = ioUtils.HandleRecovery(recover()) }()
	if m.CSeq >= 0 {
		ioUtils.WriteStringPanic(w, fmt.Sprintf("CSeq: %d\r\n", m.CSeq), &n)
	}
	if m.Session != "" {
		ioUtils.WriteStringPanic(w, fmt.Sprintf("Session: %s\r\n", m.Session), &n)
	}
	if m.ContentType != "" {
		ioUtils.WriteStringPanic(w, fmt.Sprintf("Content-Type: %s\r\n", m.ContentType), &n)
	}
	var contentLength int
	for _, content := range m.Body {
		contentLength += len(content)
	}
	if contentLength > 0 || m.ContentType != "" {
		ioUtils.WriteStringPanic(w, fmt.Sprintf("Content-Length: %d\r\n", contentLength), &n)
	}
	for name, values := range m.Header {
		if len(values) > 0 {
			ioUtils.WriteStringPanic(w, fmt.Sprintf("%s: %s\r\n", name, values[0]), &n)
		}
	}
	ioUtils.WriteStringPanic(w, "\r\n", &n)
	for _, content := range m.Body {
		ioUtils.WritePanic(w, content, &n)
	}
	return
}
