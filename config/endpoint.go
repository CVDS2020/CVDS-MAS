package config

type Endpoint struct {
	Type string       `yaml:"type" json:"type" default:"http"`
	Http EndpointHttp `yaml:"http" json:"http"`
}
