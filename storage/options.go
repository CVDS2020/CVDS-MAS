package storage

import (
	"gitee.com/sy_183/common/log"
	"time"
)

// An Option configures a Channel.
type Option interface {
	Apply(c Channel) any
}

// OptionFunc wraps a func, so it satisfies the Option interface.
type OptionFunc func(c Channel) any

func (f OptionFunc) Apply(c Channel) any {
	return f(c)
}

type OptionCustom func(c Channel)

func (f OptionCustom) Apply(c Channel) any {
	f(c)
	return nil
}

func WithCover(cover time.Duration) Option {
	return OptionCustom(func(c Channel) {
		c.SetCover(cover)
	})
}

func WithFields(fields map[string]any) Option {
	return OptionCustom(func(c Channel) {
		for name, value := range fields {
			c.SetField(name, value)
		}
	})
}

func WithLogger(logger *log.Logger) Option {
	return OptionCustom(func(c Channel) {
		c.SetLogger(logger)
	})
}
