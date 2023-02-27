package allocator

import (
	"errors"
	"fmt"
	"testing"
)

func TestBitMap(t *testing.T) {
	bm := NewBitMap(1048576)
	for i := 0; i < 110; i++ {
		fmt.Println(bm.Alloc())
	}
	for i := 0; i < 11; i++ {
		fmt.Println(bm.Free(int64(i)))
	}
	for i := 0; i < 110; i++ {
		fmt.Println(bm.Alloc())
	}
}

func testFunc() (err error) {
	defer func() {
		if err != nil {
			fmt.Println(err.Error())
		}
	}()
	return errors.New("hello")
}

func TestDefer(t *testing.T) {
	testFunc()
}
