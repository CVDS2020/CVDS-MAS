package file

import "gitee.com/sy_183/cvds-mas/storage"

// An StorageOption configures a StorageV1.
type StorageOption interface {
	apply(s *Storage) any
}

// storageOptionFunc wraps a func, so it satisfies the StorageV1Option interface.
type storageOptionFunc func(s *Storage) any

func (f storageOptionFunc) apply(s *Storage) any {
	return f(s)
}

func WithStorageDirectory(directory string) StorageOption {
	return storageOptionFunc(func(s *Storage) any {
		s.directory = directory
		return nil
	})
}

func WithChannelOptions(options ...storage.ChannelOption) StorageOption {
	return storageOptionFunc(func(s *Storage) any {
		s.channelOptions = append(s.channelOptions, options...)
		return nil
	})
}
