package config

import "time"

type EndpointHttp struct {
	Addr    string        `yaml:"addr" json:"addr" default:"localhost:18080"`
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}
