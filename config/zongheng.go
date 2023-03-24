package config

import "time"

type ZongHeng struct {
	Enable        bool          `yaml:"enable" json:"enable"`
	Endpoint      Endpoint      `yaml:"endpoint" json:"endpoint"`
	DB            DB            `yaml:"db" json:"db"`
	CheckInterval time.Duration `yaml:"check-interval" json:"check-interval" default:"5s"`
	SyncInterval  time.Duration `yaml:"sync-interval" json:"sync-interval"`
}
