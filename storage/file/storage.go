package file

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/cvds-mas/config"
	dbPkg "gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/storage"
	"gitee.com/sy_183/cvds-mas/storage/meta"
	"path"
	"time"
)

type Storage struct {
	channelOptions []storage.Option
	directory      string
}

func NewStorage() *Storage {
	cfg := config.StorageFileConfig()
	s := &Storage{directory: cfg.Directory}

	if cfg.FileMaxSize != 0 {
		s.channelOptions = append(s.channelOptions, WithFileSize(cfg.FileMaxSize))
	}
	if cfg.FileDuration != 0 {
		s.channelOptions = append(s.channelOptions, WithFileDuration(cfg.FileDuration))
	}

	if cfg.MaxDeletingFiles != 0 {
		s.channelOptions = append(s.channelOptions, WithMaxDeletingFiles(cfg.MaxDeletingFiles))
	}
	if cfg.CheckDeleteInterval != 0 {
		s.channelOptions = append(s.channelOptions, WithCheckDeleteInterval(cfg.CheckDeleteInterval))
	}

	mos := []meta.Option{meta.WithDBManager(dbPkg.NewDBManager(nil, cfg.Meta.DB.Mysql.DSN()))}
	if cfg.Meta.EarliestIndexesCacheSize != 0 {
		mos = append(mos, meta.WithEarliestIndexesCacheSize(cfg.Meta.EarliestIndexesCacheSize))
	}
	if cfg.Meta.NeedCreatedIndexesCacheSize != 0 {
		mos = append(mos, meta.WithNeedCreatedIndexesCacheSize(cfg.Meta.NeedCreatedIndexesCacheSize))
	}
	if cfg.Meta.MaxFiles != 0 {
		mos = append(mos, meta.WithMaxFiles(cfg.Meta.MaxFiles))
	}
	s.channelOptions = append(s.channelOptions, WithMetaManagerConfig(meta.ProvideDBMetaManager, mos...))

	return s
}

func (s *Storage) NewChannel(name string, cover time.Duration, options ...storage.Option) (storage.Channel, error) {
	ops := append(append([]storage.Option{WithDirectory(path.Join(s.directory, name))}, s.channelOptions...), options...)
	if cover > 0 {
		ops = append(ops, storage.WithCover(cover))
	}
	c := NewChannel(name, ops...)
	const loggerConfigReloadedCallbackId = "loggerConfigReloadedCallbackId"
	c.OnStarting(func(lifecycle.Lifecycle) {
		// 配置通道日志
		config.InitModuleLogger(c, ChannelModule, c.DisplayName())
		c.SetField(loggerConfigReloadedCallbackId, config.RegisterLoggerConfigReloadedCallback(c, ChannelModule, c.DisplayName()))

		// 配置通道元数据管理器日志
		metaManager := c.MetaManager()
		if dbMetaManager, is := metaManager.(*meta.DBMetaManager); is {
			dbMetaManager.OnStarting(func(lifecycle.Lifecycle) {
				config.InitModuleLogger(dbMetaManager, meta.DBMetaManagerModule, dbMetaManager.DisplayName())
				dbMetaManager.SetField(loggerConfigReloadedCallbackId,
					config.RegisterLoggerConfigReloadedCallback(dbMetaManager, meta.DBMetaManagerModule, dbMetaManager.DisplayName()))

				// 配置通道元数据数据库管理器日志
				dbManager := dbMetaManager.DBManager()
				config.InitModuleLogger(dbManager, meta.DBManagerModule, dbMetaManager.DisplayName())
				dbManager.SetField(loggerConfigReloadedCallbackId,
					config.RegisterLoggerConfigReloadedCallback(dbManager, meta.DBManagerModule, dbMetaManager.DisplayName()))
			}).OnClosed(func(lifecycle.Lifecycle, error) {
				dbManager := dbMetaManager.DBManager()
				config.UnregisterLoggerConfigReloadedCallback(dbManager.Field(loggerConfigReloadedCallbackId).(uint64))
				config.UnregisterLoggerConfigReloadedCallback(dbMetaManager.Field(loggerConfigReloadedCallbackId).(uint64))
			})
		}
	}).OnClosed(func(lifecycle.Lifecycle, error) {
		config.UnregisterLoggerConfigReloadedCallback(c.Field(loggerConfigReloadedCallbackId).(uint64))
	})
	return c, nil
}

var GetStorage = component.NewPointer(NewStorage).Get
