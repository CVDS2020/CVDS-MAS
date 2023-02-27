package sip

import (
	"encoding/json"
	"gitee.com/sy_183/common/uns"
	"sync/atomic"
)

type ErrorType struct {
	Type        int64  `json:"type"`
	Description string `json:"description"`
}

type GenericError struct {
	This error `json:"-"`
	ErrorType
	Cause error `json:"cause"`
}

func (e *GenericError) Init(this error, errType ErrorType, cause error) {
	e.This = this
	e.ErrorType = errType
	e.Cause = cause
}

func (e *GenericError) Error() string {
	if e == nil {
		return "null"
	}
	data, _ := json.MarshalIndent(e.This, "", "\t")
	return uns.BytesToString(data)
}

type SyntaxError struct {
	GenericError
	Text string `json:"text"`
}

func NewSyntaxError(errType ErrorType, cause error, text string) *SyntaxError {
	e := new(SyntaxError)
	e.Init(e, errType, cause)
	e.Text = text
	return e
}

type ValueError struct {
	GenericError
	Name string `json:"name"`
}

func NewValueError(errType ErrorType, cause error, name string) *ValueError {
	e := new(ValueError)
	e.Init(e, errType, cause)
	e.Name = name
	return e
}

type HostError struct {
	ValueError
	Host string
}

func NewHostError(errType ErrorType, cause error, host string) *HostError {
	e := new(HostError)
	e.Init(e, errType, cause)
	e.Name = "host"
	e.Host = host
	return e
}

var ErrorTypeCount int64

func iotaET() int64 {
	return atomic.AddInt64(&ErrorTypeCount, 1)
}

var (
	InitialLineSyntaxError = ErrorType{Type: iotaET(), Description: "sip initial line syntax error"}
	HeaderSyntaxError      = ErrorType{Type: iotaET(), Description: "sip header syntax error"}
	ParamsSyntaxError      = ErrorType{Type: iotaET(), Description: "sip params syntax error"}
	ParamsNameRepeatError  = ErrorType{Type: iotaET(), Description: "sip params name repeat"}
	HostFormatError        = ErrorType{Type: iotaET(), Description: "sip host format error"}
)

var (
	RequestLineValueError  = ErrorType{Type: iotaET(), Description: "sip request line value error"}
	ResponseLineValueError = ErrorType{Type: iotaET(), Description: "sip response line value error"}
	HeaderValueError       = ErrorType{Type: iotaET(), Description: "sip header value error"}
	ParamsNameError        = ErrorType{Type: iotaET(), Description: "sip params key error"}
	ParamsValueError       = ErrorType{Type: iotaET(), Description: "sip params value error"}
)

func NewHeaderValueError(cause error, name string) *ValueError {
	return NewValueError(HeaderValueError, cause, name)
}
