package storage

import "time"

type RetryConfig struct {
	// 当出现错误时最大重试次数
	MaxRetry int
	// 当出现错误时重试的间隔
	Interval time.Duration
	// 中断重试的信号通道
	Interrupter chan struct{}
}
