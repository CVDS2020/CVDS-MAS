package rtsp

import (
	"gitee.com/sy_183/common/errors"
)

type RtspError struct{ errors.Err }

func NewRtspError(err error) RtspError {
	return RtspError{Err: errors.Err{Err: err}}
}

func WrapRtspError(err error) RtspError {
	if re, is := err.(RtspError); !is {
		return NewRtspError(err)
	} else {
		return re
	}
}

type RtpError struct{ errors.Err }

func NewRtpError(err error) RtpError {
	return RtpError{Err: errors.Err{Err: err}}
}

func WrapRtpError(err error) RtpError {
	if re, is := err.(RtpError); !is {
		return NewRtpError(err)
	} else {
		return re
	}
}
