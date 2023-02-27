package config

import (
	"errors"
	"fmt"
	urlPkg "net/url"
)

type GB28181Proxy struct {
	Id          string `yaml:"id" json:"id"`
	HttpUrl     string `yaml:"http-uri" json:"http-uri" default:"http://127.0.0.1:8090/gb28181/proxy"`
	SipIp       string `yaml:"sip-ip" json:"sip-ip"`
	SipPort     int    `yaml:"sip-port" json:"sip-port"`
	SipDomain   string `yaml:"sip-domain" json:"sip-domain"`
	DisplayName string `yaml:"display-name" json:"display-name"`
}

func (p *GB28181Proxy) PostHandle() error {
	if p.Id == "" {
		return errors.New("GB28181代理ID不能为空")
	}
	if url, err := urlPkg.Parse(p.HttpUrl); err != nil {
		return err
	} else if url.Scheme != "http" && url.Scheme != "https" {
		return fmt.Errorf("无效的http url scheme(%s)", url.Scheme)
	}
	if p.SipIp == "" {
		return errors.New("GB28181代理SIP IP不能为空")
	}
	return nil
}
