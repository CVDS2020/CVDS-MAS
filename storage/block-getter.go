package storage

import (
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"sync"
	"sync/atomic"
)

type BlockGetter interface {
	lifecycle.Lifecycle

	Get() ([]byte, error)

	Seek(t int64)
}

type CachedBlockGetter struct {
	lifecycle.Lifecycle
	once atomic.Bool

	channel     Channel
	indexGetter IndexGetter
	readSession ReadSession

	cache      *container.Queue[[]byte]
	err        error
	addable    chan struct{}
	accessible chan struct{}
	cacheLock  sync.Mutex
}

//func NewCachedBlockGetter(channel Channel, indexGetter IndexGetter, blockSize int, cacheSize int) *CachedBlockGetter {
//
//}
//
//func (g *CachedBlockGetter) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
//	for {
//		select {
//		case <-g.addable:
//			limit := lock.LockGet(&g.cacheLock, func() int {
//				if g.err != nil {
//					return 0
//				}
//				return g.cache.Cap() - g.cache.Len()
//			})
//			if limit == 0 {
//				continue
//			}
//
//			g.readSession.Read()
//		}
//	}
//}
