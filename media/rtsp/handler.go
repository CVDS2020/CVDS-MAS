package rtsp

type Handler interface {
	HandleRequest(server *Server, request *Request) *Response
}
