package file

import (
	"fmt"
	"gitee.com/sy_183/cvds-mas/storage/file/context"
	"os"
	"sort"
	"testing"
)

func TestFind(t *testing.T) {
	s := []int{1, 3, 5, 6, 7, 10, 12, 13, 14, 19, 20}
	fmt.Println(sort.Find(len(s), func(i int) int {
		return 1 - s[i]
	}))
	c := make(chan int, 10)
	c <- 1
	c <- 2
	close(c)
	c <- 3
}

func TestRemove(t *testing.T) {
	if res := context.Remove(&context.RemoveRequest{File: "test.log"}); res.Err != nil {
		fmt.Println(res.Err)
	}
	file, err := os.OpenFile("test1.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Println(err, os.IsNotExist(err))
	}
	if res := context.Remove(&context.RemoveRequest{File: "test1.log"}); res.Err != nil {
		fmt.Println(res.Err)
	}
	if err := os.Mkdir("C:\\", 0644); err != nil {
		fmt.Println(err)
	}
	file.Close()
}
