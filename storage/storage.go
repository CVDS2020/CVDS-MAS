package storage

import (
	"time"
)

type Storage interface {
	NewChannel(channel string, cover time.Duration, options ...Option) (Channel, error)
}
