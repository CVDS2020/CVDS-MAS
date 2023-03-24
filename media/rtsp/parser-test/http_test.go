package main

import (
	"bufio"
	"bytes"
	"fmt"
	"gitee.com/sy_183/common/uns"
	"net/textproto"
	"strings"
	"testing"
)

func TestHttp(t *testing.T) {

	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(uns.StringToBytes(strings.ReplaceAll(`Accept: 123
Accept-Charset: 456
Accept-Encoding: 789
Accept-Language: 101112
Accept-Ranges: 131415
	11111
 1233213
Cache-Control: 161718
 3213
	12321
`, "\n", "\r\n")))))

	header, err := reader.ReadMIMEHeader()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(header)
}
