package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/uns"
	"github.com/gin-gonic/gin"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestServer(t *testing.T) {
	s := GetServer()
	closeFuture := s.AddClosedFuture(make(lifecycle.ChanFuture[error], 1)).(lifecycle.ChanFuture[error])
	s.Start()
	go func() {
		time.Sleep(3 * time.Second)
		s.Close(nil)
	}()
	<-closeFuture
	time.Sleep(time.Second)
}

func TestBind(t *testing.T) {
	model := struct {
		RTPMap map[uint8]string `json:"rtpMap"`
		URL    string           `json:"url"`
	}{}

	err := json.Unmarshal(uns.StringToBytes(`{
	"rtpMap": {
		"96": "PS",
		"98": "H264"
	},
	"url": "http://192.168.1.80:8080/api/v1/channel"
}`), &model)
	if err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(model.URL)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(u)

	fmt.Println(model)
}

func notify(url string, obj any) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "CVDSMAS-Channel-Hook")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return nil
}

func TestNotify(t *testing.T) {
	if err := notify("http://www.baidu.com", gin.H{
		"code": 200,
		"msg":  "success",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPost(t *testing.T) {
	req, err := http.NewRequest("POST", "http://www.baidu.com", nil)
	req.Header.Set("User-Agent", "CVDSMAS")
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(res)
}

func TestSever(t *testing.T) {
	listener, err := net.ListenUDP("udp", assert.Must(net.ResolveUDPAddr("udp", "0.0.0.0:5004")))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, listener)
}
