package rtsp2

import (
	"fmt"
	"gitee.com/sy_183/common/pool"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"io"
	"net/textproto"
)

type MessageI interface {
	CSeq() int

	SetCSeq(cSeq int)

	Session() string

	SetSession(session string)

	ContentType() string

	SetContentType(contentType string)

	ContentLength() int64

	AddHeader(key string, value string)

	SetHeader(key string, value string)

	GetHeader(key string) string

	DelHeader(key string)

	io.WriterTo

	pool.Reference
}

type Message struct {
	header textproto.MIMEHeader

	cSeq          int
	contentType   string
	contentLength int64
	session       string
	body          [][]byte
}

func (m *Message) WriteTo(w io.Writer) (n int64, err error) {
	defer func() { err = ioUtils.HandleRecovery(recover()) }()
	if m.cSeq >= 0 {
		ioUtils.WriteStringPanic(w, fmt.Sprintf("CSeq: %d\r\n", m.cSeq), &n)
	}
	if m.session != "" {
		ioUtils.WriteStringPanic(w, fmt.Sprintf("Session: %s\r\n", m.session), &n)
	}
	if m.contentType != "" {
		ioUtils.WriteStringPanic(w, fmt.Sprintf("Content-Type: %s\r\n", m.contentType), &n)
	}
	var contentLength int
	for _, content := range m.body {
		contentLength += len(content)
	}
	if contentLength > 0 || m.contentType != "" {
		ioUtils.WriteStringPanic(w, fmt.Sprintf("Content-Length: %d\r\n", contentLength), &n)
	}
	for name, values := range m.header {
		if len(values) > 0 {
			ioUtils.WriteStringPanic(w, fmt.Sprintf("%s: %s\r\n", name, values[0]), &n)
		}
	}
	ioUtils.WriteStringPanic(w, "\r\n", &n)
	for _, content := range m.body {
		ioUtils.WritePanic(w, content, &n)
	}
	return
}
