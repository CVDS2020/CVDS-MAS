package storage

import "gitee.com/sy_183/common/log"

// An ChannelOption configures a Channel.
type ChannelOption interface {
	Apply(c Channel) any
}

// ChannelOptionFunc wraps a func, so it satisfies the ChannelOption interface.
type ChannelOptionFunc func(c Channel) any

func (f ChannelOptionFunc) Apply(c Channel) any {
	return f(c)
}

type ChannelOptionCustom func(c Channel)

func (f ChannelOptionCustom) Apply(c Channel) any {
	f(c)
	return nil
}

func WithChannelFields(fields map[string]any) ChannelOption {
	return ChannelOptionCustom(func(c Channel) {
		for name, value := range fields {
			c.SetField(name, value)
		}
	})
}

func WithChannelLogger(logger *log.Logger) ChannelOption {
	return ChannelOptionCustom(func(c Channel) {
		if provider, is := c.(log.LoggerProvider); is {
			provider.SetLogger(logger)
		}
	})
}
