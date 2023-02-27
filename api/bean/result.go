package bean

type Result[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Err  any    `json:"err,omitempty"`
	Data T      `json:"data,omitempty"`
}
