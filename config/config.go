package config

import (
	"gitee.com/sy_183/common/config"
	logConfig "gitee.com/sy_183/common/log/config"
)

type Config struct {
	API      API               `yaml:"api" json:"api"`
	DB       DB                `yaml:"db" json:"db"`
	Endpoint Endpoint          `yaml:"endpoint" json:"endpoint"`
	GB28181  GB28181           `yaml:"gb28181" json:"gb28181"`
	Media    Media             `yaml:"media" json:"media"`
	Storage  Storage           `yaml:"storage" json:"storage"`
	Log      logConfig.Config  `yaml:"log" json:"log"`
	Service  Service           `yaml:"service" json:"service"`
	Figure   Figure            `yaml:"figure" json:"figure"`
	Localize map[string]string `yaml:"localize" json:"localize"`
	Version  string            `yaml:"version" json:"version" default:"v1.0.0"`
}

var context = config.NewContext[Config](config.ErrorCallback[Config](config.LoggerErrorCallback(defaultLogger)))

func init() {
	file := GetArgs().ConfigFile
	if file == "" {
		context.SetFilePrefix("config", config.TypeYaml, config.TypeJson)
	} else {
		context.SetFile(file, nil)
	}
}

func Context() *config.Context[Config] {
	return context
}

func GlobalConfig() *Config {
	return context.ConfigP()
}

func APIConfig() *API {
	return &GlobalConfig().API
}

func DBConfig() *DB {
	return &GlobalConfig().DB
}

func EndpointConfig() *Endpoint {
	return &GlobalConfig().Endpoint
}

func EndpointHttpConfig() *EndpointHttp {
	return &EndpointConfig().Http
}

func MediaConfig() *Media {
	return &GlobalConfig().Media
}

func GB28181Config() *GB28181 {
	return &GlobalConfig().GB28181
}

func GB28181ProxyConfig() *GB28181Proxy {
	return &GB28181Config().Proxy
}

func GB28181GatewayConfig() *GB28181Gateway {
	return GB28181Config().Gateway
}

func GB28181DBConfig() *DB {
	return &GB28181Config().DB
}

func MediaRTPConfig() *MediaRTP {
	return &MediaConfig().RTP
}

func StorageConfig() *Storage {
	return &GlobalConfig().Storage
}

func StorageFileConfig() *StorageFile {
	return &StorageConfig().File
}

func LogConfig() *logConfig.Config {
	return &GlobalConfig().Log
}

func ServiceConfig() *Service {
	return &GlobalConfig().Service
}

func FigureConfig() *Figure {
	return &GlobalConfig().Figure
}
