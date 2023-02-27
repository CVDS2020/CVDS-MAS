package file

import (
	"fmt"
	"sort"
	"testing"
)

type Range struct {
	Start int
	End   int
}

func TestRange(t *testing.T) {
	ranges := []Range{
		{Start: 10, End: 20},
		{Start: 30, End: 40},
		{Start: 50, End: 60},
		{Start: 70, End: 80},
		{Start: 90, End: 100},
		{Start: 110, End: 120},
		{Start: 130, End: 140},
		{Start: 150, End: 160},
		{Start: 170, End: 180},
		{Start: 190, End: 200},
		{Start: 210, End: 220},
	}

	start := 220
	end := 11
	if i := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].End > start
	}); i < len(ranges) {
		fmt.Println(ranges[i])
	} else {
		fmt.Println("not found")
	}

	if i := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].Start >= end
	}); i > 0 {
		fmt.Println(ranges[i-1])
	} else {
		fmt.Println("not found")
	}
}
