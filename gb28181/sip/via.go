package sip

import (
	"fmt"
	"gitee.com/sy_183/common/errors"
	"strings"
)

type Via struct {
	Host       string     `json:"host"`
	Port       int        `json:"port"`
	Transport  string     `json:"transport"`
	Protocol   string     `json:"protocol,omitempty"`
	TTL        int        `json:"ttl,omitempty"`
	MAddr      string     `json:"mAddr,omitempty"`
	Received   string     `json:"received,omitempty"`
	Branch     string     `json:"branch"`
	RPort      bool       `json:"rPort,omitempty"`
	RPortValue int        `json:"rPortValue,omitempty"`
	Params     Parameters `json:"params,omitempty"`
}

func (v *Via) Check() error {
	if v.Host == "" || v.Transport == "" || v.Branch == "" {
		return errors.NewArgumentMissing("via.host", "via.transport", "via.branch")
	}

	switch strings.ToLower(v.Transport) {
	case "udp":
	case "tcp":
	default:
		return errors.NewInvalidArgument("via.transport", fmt.Errorf("无效的传输协议(%s)", v.Transport))
	}

	return nil
}
