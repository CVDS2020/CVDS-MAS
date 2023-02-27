package config

import (
	"net"
	"strconv"
	"time"
)

type API struct {
	// http server listening host
	Host string `yaml:"host" json:"host" default:"localhost"`
	// http server listening port, default 8190
	Port int `yaml:"port" json:"port" default:"8190"`
	// http server listening address, calculate by Host and Port
	addr string

	// http server read timeout, default is http server default
	ReadTimeout time.Duration `yaml:"read-timeout" json:"read-timeout"`
	// http server read header timeout, default is http server default
	ReadHeaderTimeout time.Duration `yaml:"read-header-timeout" json:"read-header-timeout"`
	// http server write timeout, default is http server default
	WriteTimeout time.Duration `yaml:"write-timeout" json:"write_timeout"`
	// http server idle timeout, default is http server default
	IdleTimeout time.Duration `yaml:"idle-timeout" json:"idle-timeout"`
	// http server max header bytes, default is http server default
	MaxHeaderBytes int `yaml:"max-header-bytes" json:"max-header-bytes"`

	TLS struct {
		CertFile string `yaml:"cert-file" json:"cert-file"`
		KeyFile  string `yaml:"key-file" json:"key-file"`
	} `yaml:"tls" json:"tls"`

	Gin struct {
		EnableConsoleColor bool `yaml:"enable-console-color" json:"enable-console-color"`
	} `yaml:"gin" json:"gin"`
}

func (h *API) PostModify() (nc any, modify bool, err error) {
	// calculate http server listening address
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(h.Host, strconv.Itoa(h.Port)))
	if err != nil {
		return nil, false, err
	}
	h.addr = addr.String()
	return h, true, nil
}

func (h *API) GetAddr() string {
	return h.addr
}
