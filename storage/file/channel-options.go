package file

import (
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/storage"
	"time"
)

func WithDirectory(directory string) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.SetDirectory(directory)
		}
	})
}

func WithCover(cover time.Duration) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.SetCover(cover)
		}
	})
}

func WithBlockSize(size unit.Size) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.blockSize = size
		}
	})
}

func WithKeyFrameBlockSize(size unit.Size) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.keyFrameBlockSize = size
		}
	})
}

func WithMaxBlockSize(size unit.Size) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.maxBlockSize = size
		}
	})
}

func WithBlockDuration(duration time.Duration) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.blockDuration = duration
		}
	})
}

func WithMaxBlockDuration(duration time.Duration) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.maxBlockDuration = duration
		}
	})
}

func WithFileSize(size unit.Size) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.fileSize = size
		}
	})
}

func WithFileDuration(duration time.Duration) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.fileDuration = duration
		}
	})
}

func WithMaxDeletingFiles(files int) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.maxDeletingFiles = files
		}
	})
}

func WithCheckDeleteInterval(interval time.Duration) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.checkDeleteInterval = interval
		}
	})
}

func WithFileNameFormatter(formatter FileNameFormatter) storage.ChannelOption {
	return storage.ChannelOptionCustom(func(c storage.Channel) {
		if fc, is := c.(*Channel); is {
			fc.SetFileNameFormatter(formatter)
		}
	})
}

//func WithWriteBufferConfig(min, max uint, poolType string) storage.ChannelOption {
//	return storage.ChannelOptionCustom(func(c storage.Channel) {
//		if fc, is := c.(*Channel); is {
//			if min > 0 && max > 0 {
//				fc.writeBufferPool = pool.NewDynamicDataPoolExp(min, max, poolType)
//			}
//			fc.writeBufferPoolType = poolType
//		}
//	})
//}

type dbMetaManagerOptions []MetaManagerOption

func WithDBMetaManager(options ...MetaManagerOption) storage.ChannelOption {
	return storage.ChannelOptionFunc(func(c storage.Channel) any {
		return dbMetaManagerOptions(options)
	})
}
