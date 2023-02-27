package sip

import (
	"fmt"
	errPkg "gitee.com/sy_183/cvds-mas/errors"
	responsePkg "gitee.com/sy_183/cvds-mas/gb28181/sip/response"
)

type Response struct {
	StatusCode   int    `json:"statusCode"`
	ReasonPhrase string `json:"reasonPhrase,omitempty"`
	Message
}

func (r *Response) Check() error {
	if r.StatusCode == 0 {
		return errPkg.NewArgumentMissing("response.statusCode")
	}
	if r.StatusCode < responsePkg.Trying || r.StatusCode > responsePkg.SessionNotAcceptable {
		return errPkg.NewInvalidArgument("response.statusCode", fmt.Errorf("无效的SIP响应码(%d)", r.StatusCode))
	}
	if err := r.Message.Check(); err != nil {
		if e, is := err.(interface{ ReplaceParentArgument(parent string) }); is {
			e.ReplaceParentArgument("response")
		}
	}
	return nil
}
