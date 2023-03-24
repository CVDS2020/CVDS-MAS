package parser

import (
	"fmt"
	"io"
	"os"
	"testing"
)

func TestLineParser(t *testing.T) {
	lp := &LineParser{
		MaxLineSize: 1300,
	}
	file, err := os.Open("line-parser.go")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for {
		buf := make([]byte, 1)
		n, err := file.Read(buf)
		if err != nil {
			if err == io.EOF {
				return
			}
			t.Fatal(err)
		}
		chunk := buf[:n]
		for len(chunk) > 0 {
			if ok, err := lp.ParseP(chunk, &chunk); err != nil {
				fmt.Println(err)
			} else if ok {
				fmt.Println(lp.Line())
			}
		}
	}
}
