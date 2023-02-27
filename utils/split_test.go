package utils

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	fmt.Println(strings.SplitN("abc:", ":", 2))
	fmt.Println(strconv.ParseUint("123291-123", 10, 64))
}
