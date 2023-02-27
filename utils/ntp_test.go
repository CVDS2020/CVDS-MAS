package utils

import (
	"fmt"
	"gitee.com/sy_183/sdp"
	"testing"
	"time"
)

func TestNTP(t *testing.T) {
	fmt.Println(
		sdp.TimeToNTP(time.UnixMilli(1676013707813)),
		sdp.TimeToNTP(time.UnixMilli(1676014362049)),
	)
}
