package main

import (
	"fmt"
	urlPkg "net/url"
	"testing"
)

func TestURL(t *testing.T) {
	raw := "rtsp://admin:css66018@192.168.1.123:554/Streaming/Channels/101"
	url, err := urlPkg.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(url.Path)
}
