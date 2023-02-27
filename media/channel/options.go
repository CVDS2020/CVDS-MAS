package channel

import "gitee.com/sy_183/common/log"

type Option interface {
	Apply(channel *Channel)
}

type OptionFunc func(channel *Channel)

func (f OptionFunc) Apply(channel *Channel) {
	f(channel)
}

func WithFields(fields map[string]any) Option {
	return OptionFunc(func(channel *Channel) {
		for name, value := range fields {
			channel.SetField(name, value)
		}
	})
}

func WithLogger(logger *log.Logger) Option {
	return OptionFunc(func(channel *Channel) {
		channel.SetLogger(logger)
	})
}
