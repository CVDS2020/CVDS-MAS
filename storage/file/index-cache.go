package file

import (
	"fmt"
	"gitee.com/sy_183/common/container"
	"sort"
)

const DefaultIndexCacheSize = 1024

type IndexCache struct {
	channelInfo *ChannelInfo

	queue *container.Queue[Index]

	first *Index
	last  *Index
}

func NewIndexCache(channelInfo *ChannelInfo, cap int) *IndexCache {
	if cap < 1 {
		cap = DefaultIndexCacheSize
	}
	return &IndexCache{
		channelInfo: channelInfo,
		queue:       container.NewQueue[Index](cap),
	}
}

func (c *IndexCache) Len() int {
	return c.queue.Len()
}

func (c *IndexCache) Cap() int {
	return c.queue.Cap()
}

func (c *IndexCache) First() *Index {
	return c.first
}

func (c *IndexCache) Last() *Index {
	return c.last
}

func (c *IndexCache) Index(i int) *Index {
	return c.queue.Pointer(i)
}

func (c *IndexCache) FindBySeq(seq uint64) (i int, found bool) {
	if c.first == nil || seq < c.first.Seq {
		return 0, false
	} else if seq > c.last.Seq {
		return c.queue.Len(), false
	}
	if seq == c.last.Seq {
		return c.queue.Len() - 1, true
	} else if seq == c.first.Seq {
		return 0, true
	}
	i = sort.Search(c.queue.Len(), func(i int) bool {
		return c.queue.Get(i).Seq >= seq
	})
	return i, c.queue.Get(i).Seq == seq
}

func (c *IndexCache) FindByTime(timestamp int64) (i int, found bool) {
	if c.first == nil || timestamp < c.first.Start {
		return 0, false
	} else if timestamp >= c.last.End {
		return c.queue.Len(), false
	}
	return sort.Search(c.queue.Len(), func(i int) bool {
		return c.queue.Pointer(i).End > timestamp
	}), true
}

func (c *IndexCache) Add(index *Index, autoRemove bool) bool {
	if c.last != nil {
		if index.Seq <= c.last.Seq {
			panic(fmt.Errorf("index sequence(%d) less than or equal to the previous(%d)", index.Seq, c.last.Seq))
		}
		if index.Start < c.last.End {
			locationName, locationOffset := c.channelInfo.Location, c.channelInfo.LocationOffset
			panic(fmt.Errorf("index start time(%s) less then previous(%s) end time",
				formatUnixMilli(index.Start, locationName, locationOffset),
				formatUnixMilli(c.last.End, locationName, locationOffset)),
			)
		}
	}
	if c.queue.Len() >= c.queue.Cap() {
		if autoRemove {
			c.queue.Pop()
		} else {
			return false
		}
	}
	c.queue.Push(*index)
	c.last = c.queue.TailPointer()
	if c.first == nil {
		c.first = c.last
	}
	return true
}

func (c *IndexCache) Remove() *Index {
	if c.first == nil {
		return nil
	}
	index := new(Index)
	*index, _ = c.queue.Pop()
	return index
}
