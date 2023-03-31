package file

import (
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/storage"
	"gitee.com/sy_183/cvds-mas/storage/meta"
	"time"
)

func WithDirectory(directory string) storage.Option {
	return storage.OptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.SetDirectory(directory)
		}
	})
}

func WithFileSize(size unit.Size) storage.Option {
	return storage.OptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.fileSize = size
		}
	})
}

func WithFileDuration(duration time.Duration) storage.Option {
	return storage.OptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.fileDuration = duration
		}
	})
}

func WithFileNameFormatter(formatter storage.FileNameFormatter) storage.Option {
	return storage.OptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.SetFileNameFormatter(formatter)
		}
	})
}

func WithCheckDeleteInterval(interval time.Duration) storage.Option {
	return storage.OptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.checkDeleteInterval = interval
		}
	})
}

func WithWriteBufferPoolConfig(writeBufferSize uint, provider pool.PoolProvider[*Buffer]) storage.Option {
	return storage.OptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.writeBufferSize = writeBufferSize
			fc.writeBufferPoolProvider = provider
		}
	})
}

func WithMaxSyncReqs(syncReqs int) storage.Option {
	return storage.OptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			if syncReqs < MinMaxSyncReqs {
				syncReqs = MinMaxSyncReqs
			}
			fc.syncReqs = container.NewQueue[syncRequest](syncReqs)
		}
	})
}

func WithMaxDeletingFiles(files int) storage.Option {
	return storage.OptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			if files < MinMaxDeletingFiles {
				files = MinMaxDeletingFiles
			}
			fc.deleteFiles = container.NewQueue[*meta.File](files)
		}
	})
}

type metaManagerConfig struct {
	provider meta.ManagerProvider
	options  []meta.Option
}

func (c metaManagerConfig) build(ch storage.Channel) meta.Manager {
	return c.provider(ch, c.options...)
}

func WithMetaManagerConfig(provider meta.ManagerProvider, options ...meta.Option) storage.Option {
	return storage.OptionFunc(func(c storage.Channel) any {
		return metaManagerConfig{provider: provider, options: options}
	})
}
