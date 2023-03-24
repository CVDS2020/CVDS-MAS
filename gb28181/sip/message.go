package sip

import "gitee.com/sy_183/common/errors"

type Message struct {
	LocalIp     string   `json:"localIp,omitempty"`
	LocalPort   int      `json:"localPort,omitempty"`
	RemoteIp    string   `json:"remoteIp,omitempty"`
	RemotePort  int      `json:"remotePort,omitempty"`
	From        From     `json:"from"`
	To          To       `json:"to"`
	Via         []Via    `json:"via"`
	CSeq        uint32   `json:"cSeq"`
	CallId      string   `json:"callId"`
	MaxForwards int      `json:"maxForwards,omitempty"`
	Contact     *Contact `json:"contact,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	UserAgent   string   `json:"userAgent,omitempty"`
	UserAgents  []string `json:"userAgents,omitempty"`
	ContentType string   `json:"contentType,omitempty"`
	Content     string   `json:"content,omitempty"`
}

func (m *Message) Check() error {
	if m.From.Address.URI == "" ||
		m.To.Address.URI == "" ||
		len(m.Via) == 0 ||
		m.CallId == "" {
		return errors.NewArgumentMissing(
			"message.from",
			"message.to",
			"message.via",
			"message.callId",
		)
	}

	for i := range m.Via {
		if err := m.Via[i].Check(); err != nil {
			if e, is := err.(interface{ AddParentArgument(parent string) }); is {
				e.AddParentArgument("response")
			}
		}
	}

	return nil
}
