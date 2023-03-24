package start_code

import (
	"fmt"
	"io"
	"os"
	"testing"
)

func TestFinder(t *testing.T) {
	file, err := os.Open("C:\\Users\\suy\\Videos\\AQUAMAN\\VIDEO-H264-1080P-4M.h264")
	if err != nil {
		t.Fatal(err)
	}
	finder := StartCodeFinder{}
	buf := make([]byte, 64)
	for {
		n, err := file.Read(buf)
		if err != nil {
			if err == io.EOF {
				return
			}
			t.Fatal(err)
		}
		p := buf[:n]
		for len(p) > 0 {
			startCode, remain := finder.Find(p)
			p = remain
			if startCode[2] == 1 && startCode[3]&0b00011111 == 5 {
				fmt.Printf("[0x%02x 0x%02x 0x%02x 0x%02x]\n", startCode[0], startCode[1], startCode[2], startCode[3]&0b00011111)
			}
		}
	}
}
