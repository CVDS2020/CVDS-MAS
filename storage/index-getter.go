package storage

import (
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/utils"
	"io"
	"sort"
	"sync"
	"sync/atomic"
)

type IndexGetter interface {
	lifecycle.Lifecycle

	Get() (Index, error)

	Seek(t int64)
}

const (
	DefaultCachedIndexGetterBufferSize = 64
	DefaultCachedIndexGetterCacheSize  = 4096
	MinCachedIndexGetterCacheSize      = 64
)

type CachedIndexGetter struct {
	lifecycle.Lifecycle
	once atomic.Bool

	channel Channel

	startTime int64
	endTime   int64

	buffer     Indexes
	cache      *container.Queue[Index]
	err        error
	addable    chan struct{}
	accessible chan struct{}
	cacheLock  sync.Mutex

	cur     int
	curTime int64
}

func NewCachedIndexGetter(channel Channel, startTime, endTime int64, bufferSize int, cacheSize int) *CachedIndexGetter {
	if bufferSize <= 0 {
		bufferSize = DefaultCachedIndexGetterBufferSize
	}
	if cacheSize == 0 {
		cacheSize = DefaultCachedIndexGetterCacheSize
	}
	if cacheSize < MinCachedIndexGetterCacheSize {
		cacheSize = MinCachedIndexGetterCacheSize
	}
	if cacheSize < bufferSize {
		cacheSize = bufferSize
	}
	getter := &CachedIndexGetter{
		channel: channel,

		startTime: startTime,
		endTime:   endTime,

		buffer: channel.MakeIndexes(bufferSize),
		cache:  container.NewQueue[Index](cacheSize),

		addable:    make(chan struct{}, 1),
		accessible: make(chan struct{}, 1),

		curTime: startTime,
	}
	getter.Lifecycle = lifecycle.NewWithInterruptedRun(getter.start, getter.run)
	return getter
}

func (g *CachedIndexGetter) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if !g.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError("")
	}
	g.addable <- struct{}{}
	return nil
}

func (g *CachedIndexGetter) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	for {
		select {
		case <-g.addable:
			var lastIndex Index
			var start int64
			limit := lock.LockGet(&g.cacheLock, func() int {
				if g.err != nil {
					return 0
				}
				if tail, has := g.cache.Tail(); has {
					lastIndex = tail
					start = lastIndex.End()
				} else {
					start = g.curTime
				}
				return g.cache.Cap() - g.cache.Len()
			})
			threshold := g.cache.Cap() >> 1
			if threshold > g.buffer.Cap() {
				threshold = g.buffer.Cap()
			}
			if limit < threshold {
				continue
			}
			if limit > g.buffer.Cap() {
				limit = g.buffer.Cap()
			}

			indexes, err := g.channel.FindIndexes(start, g.endTime, limit, g.buffer)
			if err != nil {
				if lock.LockGet(&g.cacheLock, func() (accessible bool) {
					if tail, has := g.cache.Tail(); has {
						if tail.End() != start {
							return false
						}
					} else if g.curTime != start {
						return false
					}
					g.err = err
					return false
				}) {
					utils.ChanTryPush(g.accessible, struct{}{})
				}
			}

			var checkStartSeq uint64
			var checkStartTime int64
			if lastIndex != nil {
				checkStartSeq = lastIndex.Seq()
				checkStartTime = lastIndex.End()
			}
			indexChecker := NewIndexChecker(checkStartTime, 0, checkStartSeq, 0)
			if lock.LockGet(&g.cacheLock, func() (accessible bool) {
				if tail, has := g.cache.Tail(); has {
					if tail.End() != start {
						return false
					}
				} else if g.curTime != start {
					return false
				}
				for i := 0; i < indexes.Len(); i++ {
					index := indexes.Get(i)
					if indexChecker.Check(index) {
						g.cache.Push(index.Clone())
					}
				}
				if limit > indexes.Len() {
					g.err = io.EOF
				}
				return true
			}) {
				utils.ChanTryPush(g.accessible, struct{}{})
			}
		case <-interrupter:
			close(g.accessible)
			return nil
		}
	}
}

func (g *CachedIndexGetter) Get() (Index, error) {
	for {
		addable := true
		index, err := lock.LockGetDouble(&g.cacheLock, func() (Index, error) {
			if g.cache.Len() == g.cur {
				addable = false
				return nil, g.err
			}
			index := g.cache.Get(g.cur).Clone()
			g.curTime = index.End()
			if g.cur < g.cache.Cap()>>1 {
				g.cur++
				return index, nil
			}
			g.cache.Pop()
			return index, nil
		})
		if addable {
			utils.ChanTryPush(g.addable, struct{}{})
		}
		if index != nil || err != nil {
			return index, err
		}
		if _, ok := <-g.accessible; !ok {
			return nil, lifecycle.NewInterruptedError("", "")
		}
	}
}

func (g *CachedIndexGetter) popCache() (popped bool) {
	maxHistory := g.cache.Cap() >> 1
	for g.cur > maxHistory {
		if !popped {
			popped = true
		}
		g.cache.Pop()
		g.cur--
	}
	return
}

func (g *CachedIndexGetter) Seek(t int64) {
	if t < g.startTime {
		t = g.startTime
	} else if t > g.endTime {
		t = g.endTime
	}
	if addable, accessible := lock.LockGetDouble(&g.cacheLock, func() (addable bool, accessible bool) {
		if g.curTime == t {
			// 定位时间就是当前时间，不需要执行任何操作
			return false, false
		}
		g.curTime = t
		if g.curTime == g.endTime {
			// 定位到了索引范围的结尾
			if g.err == io.EOF {
				g.cur = g.cache.Len()
				g.popCache()
				return false, false
			}
			defer func() { g.err = io.EOF }()
			if g.cache.Len() == 0 {
				if g.err != nil {
					return false, false
				}
				return false, true
			}
			g.cache.Clear()
			g.cur = 0
			return false, false
		}

		if g.cache.Len() > 0 {
			firstIndex, _ := g.cache.Head()
			lastIndex, _ := g.cache.Tail()
			if g.curTime < firstIndex.Start() || g.curTime >= lastIndex.End() {
				// 定位的位置不在缓存中，清除缓存
				g.cache.Clear()
				g.err = nil
				g.cur = 0
				return true, false
			}

			// 定位的位置在缓存中
			g.cur = sort.Search(g.cache.Len(), func(i int) bool {
				return g.cache.Get(i).End() > g.curTime
			})
			if g.err != nil {
				g.popCache()
				return false, false
			}
			return g.popCache(), false
		}

		return false, false
	}); addable {
		utils.ChanTryPush(g.addable, struct{}{})
	} else if accessible {
		defer func() { recover() }()
		utils.ChanTryPush(g.accessible, struct{}{})
	}
}
