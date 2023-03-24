package endpoint

import "fmt"

type ResultStatusError struct {
	Code int
	Msg  string
}

func (e *ResultStatusError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Msg != "" {
		return fmt.Sprintf("请求返回错误代码(%d), 错误原因: %s", e.Code, e.Msg)
	} else {
		return fmt.Sprintf("请求返回错误代码(%d)", e.Code)
	}
}
