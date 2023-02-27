package errors

import (
	"errors"
	"fmt"
	"strings"
)

var (
	StorageConfigAddedError         = errors.New("存储配置已经添加到数据库")
	StorageConfigModifiedError      = errors.New("存储配置名称不可以修改")
	StorageConfigNotAvailableError  = errors.New("存储配置不可用")
	StorageConfigDBTaskFullError    = errors.New("存储配置更新任务已满")
	StorageConfigRelateChannelError = errors.New("存储配置与通道存在关联")
)

var (
	StorageConfigManagerNotAvailableError = errors.New("存储配置管理器不可用")
)

type InvalidArgumentError struct {
	Argument string
	Err      error
}

func (e *InvalidArgumentError) Error() string {
	if e == nil {
		return "<nil>"
	} else if e.Argument == "" {
		return "参数解析错误：" + e.Err.Error()
	} else {
		return fmt.Sprintf("参数解析错误(%s)：%s", e.Argument, e.Err.Error())
	}
}

type ArgumentMissingError struct {
	Arguments []string
}

func (e *ArgumentMissingError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return "缺少必要的参数(" + strings.Join(e.Arguments, ",") + ")"
}

type InvalidPortError struct {
	Port int32
}

type HttpStatusError struct {
	StatusCode int
	Status     string
}

func (e *HttpStatusError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Status != "" {
		return fmt.Sprintf("HTTP请求返回错误代码(%s)", e.Status)
	} else {
		return fmt.Sprintf("HTTP请求返回错误代码(%d)", e.StatusCode)
	}
}

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

type SipResponseError struct {
	StatusCode   int
	ReasonPhrase string
}

func (e *SipResponseError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.ReasonPhrase != "" {
		return fmt.Sprintf("SIP请求返回错误代码(%d), 错误原因: %s", e.StatusCode, e.ReasonPhrase)
	} else {
		return fmt.Sprintf("SIP请求返回错误代码(%d)", e.StatusCode)
	}
}
