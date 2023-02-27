package file

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/storage"
	"path"
	"strings"
	"time"
)

type Storage struct {
	channelOptions []storage.ChannelOption
	directory      string
}

func NewStorage(options ...StorageOption) *Storage {
	s := &Storage{}
	for _, option := range options {
		option.apply(s)
	}
	return s
}

func (s *Storage) NewChannel(channel string, cover time.Duration, options ...storage.ChannelOption) (storage.Channel, error) {
	ops := append([]storage.ChannelOption{
		WithDirectory(path.Join(s.directory, channel)),
	}, s.channelOptions...)
	ops = append(ops, options...)
	if cover > 0 {
		ops = append(ops, WithCover(cover))
	}
	c := NewChannel(channel, ops...)
	return c, nil
}

var GetStorage = component.NewPointer(func() *Storage {
	cfg := config.StorageFileConfig()
	var cos []storage.ChannelOption
	if cfg.BlockSize != 0 {
		cos = append(cos, WithBlockSize(cfg.BlockSize))
	}
	if cfg.KeyFrameBlockSize != 0 {
		cos = append(cos, WithKeyFrameBlockSize(cfg.KeyFrameBlockSize))
	}
	if cfg.MaxBlockSize != 0 {
		cos = append(cos, WithMaxBlockSize(cfg.MaxBlockSize))
	}

	if cfg.BlockDuration != 0 {
		cos = append(cos, WithBlockDuration(cfg.BlockDuration))
	}
	if cfg.MaxBlockDuration != 0 {
		cos = append(cos, WithMaxBlockDuration(cfg.MaxBlockDuration))
	}

	if cfg.FileMaxSize != 0 {
		cos = append(cos, WithFileSize(cfg.FileMaxSize))
	}
	if cfg.FileDuration != 0 {
		cos = append(cos, WithFileDuration(cfg.FileDuration))
	}

	if cfg.MaxDeletingFiles != 0 {
		cos = append(cos, WithMaxDeletingFiles(cfg.MaxDeletingFiles))
	}
	if cfg.CheckDeleteInterval != 0 {
		cos = append(cos, WithCheckDeleteInterval(cfg.CheckDeleteInterval))
	}

	var mos []MetaManagerOption
	switch strings.ToLower(cfg.Meta.DB.Type) {
	case "mysql":
		mos = append(mos, WithDBMetaManagerDSN(cfg.Meta.DB.Mysql.DSN()))
	}
	if cfg.Meta.EarliestIndexesCacheSize != 0 {
		mos = append(mos, WithMetaManagerEarliestIndexesCacheSize(cfg.Meta.EarliestIndexesCacheSize))
	}
	if cfg.Meta.NeedCreatedIndexesCacheSize != 0 {
		mos = append(mos, WithMetaManagerNeedCreatedIndexesCacheSize(cfg.Meta.NeedCreatedIndexesCacheSize))
	}
	if cfg.Meta.MaxFiles != 0 {
		mos = append(mos, WithMetaManagerMaxFiles(cfg.Meta.MaxFiles))
	}

	if len(mos) > 0 {
		cos = append(cos, WithDBMetaManager(mos...))
	}

	var ops []StorageOption
	if cfg.Directory != "" {
		ops = append(ops, WithStorageDirectory(cfg.Directory))
	}
	if len(cos) > 0 {
		ops = append(ops, WithChannelOptions(cos...))
	}
	return NewStorage(ops...)
}).Get
