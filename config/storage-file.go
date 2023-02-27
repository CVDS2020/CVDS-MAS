package config

import (
	"gitee.com/sy_183/common/unit"
	"time"
)

type StorageFile struct {
	Directory string `yaml:"directory" json:"directory"`

	BlockSize         unit.Size `yaml:"block-size" json:"block-size" default:"1MiB"`
	KeyFrameBlockSize unit.Size `yaml:"key-frame-block-size" json:"key-frame-block-size" default:"1.5MiB"`
	MaxBlockSize      unit.Size `yaml:"max-block-size" json:"max-block-size" default:"2MiB"`

	BlockDuration    time.Duration `yaml:"block-duration" json:"block-duration" default:"4s"`
	MaxBlockDuration time.Duration `yaml:"max-block-duration" json:"max-block-duration" default:"8s"`

	FileMaxSize  unit.Size     `yaml:"file-max-size" json:"file-max-size" default:"1GiB"`
	FileDuration time.Duration `yaml:"file-duration" yaml:"file-duration" default:"10m"`

	MaxDeletingFiles    int           `yaml:"max-deleting-files" json:"max-deleting-files"`
	CheckDeleteInterval time.Duration `yaml:"check-delete-interval" json:"check-delete-interval"`

	//WriteBufferMinSize  unit.Size     `yaml:"write-buffer-min-size" json:"write-buffer-min-size"`
	//WriteBufferMaxSize  unit.Size     `yaml:"write-buffer-max-size" json:"write-buffer-max-size"`
	//WriteBufferPoolType string        `yaml:"write-buffer-pool-type" json:"write-buffer-pool-type" default:"slice"`
	//MetaDB DB `yaml:"meta-db" json:"meta-db"`
	Meta struct {
		DB                          DB  `yaml:"db" json:"db"`
		EarliestIndexesCacheSize    int `yaml:"earliest-indexes-cache-size" json:"earliest-indexes-cache-size"`
		NeedCreatedIndexesCacheSize int `yaml:"need-created-indexes-cache-size" json:"need-created-indexes-cache-size"`
		MaxFiles                    int `yaml:"max-files" json:"max-files"`
	} `yaml:"meta" json:"meta"`
}

func (f *StorageFile) PreModify() (nc any, modified bool) {
	f.Meta.DB.Type = "mysql"
	f.Meta.DB.Mysql.DBName = "cvdsrec"
	return f, true
}
