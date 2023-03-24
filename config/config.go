package config

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	logConfig "gitee.com/sy_183/common/log/config"
)

type Config struct {
	API      API               `yaml:"api" json:"api"`
	GB28181  GB28181           `yaml:"gb28181" json:"gb28181"`
	ZongHeng ZongHeng          `yaml:"zongheng" json:"zongheng"`
	Media    Media             `yaml:"media" json:"media"`
	Storage  Storage           `yaml:"storage" json:"storage"`
	Log      logConfig.Config  `yaml:"log" json:"log"`
	Service  Service           `yaml:"service" json:"service"`
	Figure   Figure            `yaml:"figure" json:"figure"`
	Localize map[string]string `yaml:"localize" json:"localize"`
	Version  string            `yaml:"version" json:"version" default:"v1.0.0"`
}

var context = config.NewContext[Config](config.ErrorCallback[Config](config.LoggerErrorCallback(defaultLogger)))

var initContext = component.NewPointer(func() *config.Context[Config] {
	file := GetArgs().ConfigFile
	if file == "" {
		context.SetFilePrefix("config", config.TypeYaml, config.TypeJson)
	} else {
		context.SetFile(file, nil)
	}
	return context
}).Get

func Context() *config.Context[Config] {
	return context
}

func GlobalConfig() *Config {
	return initContext().ConfigP()
}

func APIConfig() *API {
	return &GlobalConfig().API
}

func GB28181Config() *GB28181 {
	return &GlobalConfig().GB28181
}

func GB28181DBConfig() *DB {
	return &GB28181Config().DB
}

func GB28181ProxyConfig() *GB28181Proxy {
	return &GB28181Config().Proxy
}

func ZongHengConfig() *ZongHeng {
	return &GlobalConfig().ZongHeng
}

func ZongHengDBConfig() *DB {
	return &ZongHengConfig().DB
}

func ZongHengEndpointConfig() *Endpoint {
	return &ZongHengConfig().Endpoint
}

func MediaConfig() *Media {
	return &GlobalConfig().Media
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
