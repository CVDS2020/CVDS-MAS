package storage

import (
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/utils"
	"io"
	"sync"
)

type indexContext struct {
	Index
	err error
}

type IndexGetter struct {
	lifecycle.Lifecycle

	channel   Channel
	cache     *container.Queue[indexContext]
	buffer    Indexes
	addable   chan struct{}
	removable chan struct{}
	cacheLock sync.Mutex

	curTime   int64
	startTime int64
	endTime   int64
}

func (g *IndexGetter) getFindContext() (need int, curTime int64) {
	need = g.cache.Cap() - g.cache.Len()
	if need > 0 && g.cache.TailPointer().err != nil {
		return 0, g.curTime
	}
	return need, g.curTime
}

func (g *IndexGetter) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	for {
		need, curTime := lock.LockGetDouble(&g.cacheLock, g.getFindContext)

		for need == 0 {
			select {
			case <-g.addable:
				need, curTime = lock.LockGetDouble(&g.cacheLock, g.getFindContext)
			case <-interrupter:
				close(g.removable)
				return nil
			}
		}

		indexes, err := g.channel.FindIndexes(curTime, g.endTime, need, g.buffer)
		if err != nil {
			if lock.LockGet(&g.cacheLock, func() (pushed bool) {
				if cur := g.curTime; cur != curTime {
					return false
				}
				g.cache.Push(indexContext{err: err})
				return true
			}) {
				utils.ChanTryPush(g.removable, struct{}{})
			}
		}

		if lock.LockGet(&g.cacheLock, func() (pushed bool) {
			if cur := g.curTime; cur != curTime {
				return false
			}
			for i := 0; i < indexes.Len(); i++ {
				g.cache.Push(indexContext{Index: indexes.Get(i)})
			}
			return indexes.Len() > 0
		}) {
			utils.ChanTryPush(g.removable, struct{}{})
		}
	}
}

func (g *IndexGetter) Get() (Index, error) {
	for {
		var popped bool
		index, err := lock.LockGetDouble(&g.cacheLock, func() (Index, error) {
			if ctx, exist := g.cache.Head(); exist {
				if ctx.err != io.EOF {
					popped = true
					g.cache.Pop()
				}
				return ctx.Index, ctx.err
			}
			return nil, nil
		})
		if popped {
			utils.ChanTryPush(g.addable, struct{}{})
		}
		if index != nil || err != nil {
			return index, err
		}
		if _, ok := <-g.removable; !ok {
			return nil, lifecycle.NewInterruptedError("", "")
		}
	}
}

func (g *IndexGetter) Seek(t int64) {
	if t < g.startTime {
		t = g.startTime
	} else if t > g.endTime {
		t = g.endTime
	}
	if popped, pushed := lock.LockGetDouble(&g.cacheLock, func() (popped, pushed bool) {
		if g.curTime == t {
			return false, false
		}
		g.curTime = t
		if g.curTime == g.endTime {
			g.cache.Clear()
			g.cache.Push(indexContext{err: io.EOF})
			return false, true
		}
		var lastEnd int64
		for {
			if ctx, exist := g.cache.Head(); !exist {
				return popped, false
			} else {
				if ctx.err != nil {
					if lastEnd == 0 {
						// 如果缓存中的第一个索引的起始时间在定位时间之后，则说明向前定位，此时需要
						// 清除所有的索引缓存并重新加载
						lastEnd = ctx.Index.StartTime()
						if g.curTime < lastEnd {
							g.cache.Clear()
							return true, false
						}
					}
					if g.curTime >= lastEnd && g.curTime < ctx.Index.EndTime() {
						// 在此索引之后(包括此索引)的缓存不需要重新加载
						return popped, false
					}
					lastEnd = ctx.Index.EndTime()
				}
				g.cache.Pop()
				popped = true
			}
		}
	}); popped {
		utils.ChanTryPush(g.addable, struct{}{})
	} else if pushed {
		utils.ChanTryPush(g.removable, struct{}{})
	}
}
