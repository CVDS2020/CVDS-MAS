package sip

import (
	errPkg "gitee.com/sy_183/cvds-mas/errors"
)

type Request struct {
	URI string `json:"uri"`
	Message
}

func (r *Request) Check() error {
	if r.URI == "" {
		return errPkg.NewArgumentMissing("request.URI")
	}
	if err := r.Message.Check(); err != nil {
		if e, is := err.(interface{ ReplaceParentArgument(parent string) }); is {
			e.ReplaceParentArgument("request")
		}
	}
	return nil
}
