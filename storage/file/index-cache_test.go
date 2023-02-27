package file

import (
	"fmt"
	"gitee.com/sy_183/common/container"
	"testing"
)

func TestIndexCache(t *testing.T) {
	ic := &IndexCache{
		channelInfo: &ChannelInfo{
			Location:       "BJ",
			LocationOffset: curLocationOffset(),
		},
		queue: container.NewQueue[Index](32),
	}

	ic.Add(&Index{
		Seq:   1,
		Start: 10,
		End:   20,
	}, false)

	ic.Add(&Index{
		Seq:   3,
		Start: 20,
		End:   30,
	}, false)

	ic.Add(&Index{
		Seq:   5,
		Start: 30,
		End:   40,
	}, false)

	ic.Add(&Index{
		Seq:   6,
		Start: 50,
		End:   60,
	}, false)

	ic.Add(&Index{
		Seq:   7,
		Start: 70,
		End:   80,
	}, false)

	fmt.Println("find by sequence")
	for i := 0; i < 10; i++ {
		fmt.Print(i, " ")
		fmt.Println(ic.FindBySeq(uint64(i)))
	}

	fmt.Println("find by time")
	for i := 0; i < 100; i += 1 {
		fmt.Print(i, " ")
		fmt.Println(ic.FindByTime(int64(i)))
	}
}
