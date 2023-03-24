package main

import (
	"fmt"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/url"
	"github.com/pion/rtp"
	"log"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/base"
)

type server struct {
	stream  *gortsplib.ServerStream
	client  *gortsplib.Client
	baseURL *url.URL
}

func newServer() *server {
	s := &server{}

	// configure the server
	rs := &gortsplib.Server{
		Handler:           s,
		RTSPAddress:       ":554",
		UDPRTPAddress:     ":8000",
		UDPRTCPAddress:    ":8001",
		MulticastIPRange:  "224.1.0.0/16",
		MulticastRTPPort:  8002,
		MulticastRTCPPort: 8003,
	}

	// start server and wait until a fatal error
	log.Printf("server is ready")
	panic(rs.StartAndWait())
}

// called when a connection is opened.
func (s *server) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	log.Printf("conn opened")
}

// called when a connection is closed.
func (s *server) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	log.Printf("conn closed (%v)", ctx.Error)
}

// called when a session is opened.
func (s *server) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	log.Printf("session opened")
}

// called when a session is closed.
func (s *server) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	log.Printf("session closed")
}

// called when receiving a DESCRIBE request.
func (s *server) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (resp *base.Response, stream *gortsplib.ServerStream, err error) {
	log.Printf("describe request")

	fmt.Println(ctx.Request)
	defer func() {
		fmt.Println(resp)
	}()

	if s.stream != nil {
		return &base.Response{
			StatusCode: base.StatusOK,
		}, s.stream, nil
	}

	transport := gortsplib.TransportTCP
	c := gortsplib.Client{
		Transport: &transport,
	}

	u, err := url.Parse("rtsp://admin:css66018@192.168.1.123/Streaming/Channels/101")
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, nil
	}

	c.Start(u.Scheme, u.Host)

	medias, baseURL, _, err := c.Describe(u)
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, nil
	}

	s.client = &c
	s.baseURL = baseURL
	s.stream = gortsplib.NewServerStream(medias)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, s.stream, nil
}

// called when receiving a SETUP request.
func (s *server) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (resp *base.Response, stream *gortsplib.ServerStream, err error) {
	log.Printf("setup request")

	fmt.Println(ctx.Request)
	defer func() {
		fmt.Println(resp)
	}()

	// stream is not available yet
	if s.stream == nil {
		return &base.Response{
			StatusCode: base.StatusNotFound,
		}, nil, nil
	}

	medias := s.stream.Medias()
	for _, m := range medias {
		if m.URL()
	}

	if err := s.client.SetupAll(s.stream.Medias(), s.baseURL); err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, nil
	}

	s.client.OnPacketRTPAny(func(medi *media.Media, forma format.Format, pkt *rtp.Packet) {
		// route incoming packets to the server stream
		s.stream.WritePacketRTP(medi, pkt)
	})

	return &base.Response{
		StatusCode: base.StatusOK,
	}, s.stream, nil
}

// called when receiving a PLAY request.
func (s *server) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (resp *base.Response, err error) {
	log.Printf("play request")

	fmt.Println(ctx.Request)
	defer func() {
		fmt.Println(resp)
	}()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}
