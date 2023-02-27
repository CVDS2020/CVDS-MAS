package sip

import (
	"bytes"
	"gitee.com/sy_183/common/uns"
	"strings"
)

type Parameters map[string]string

func (ps *Parameters) CopyFrom(other Parameters) {
	ps.Clear()
	if other != nil && len(other) > 0 {
		if *ps == nil {
			*ps = make(Parameters)
		}
		for k, v := range other {
			(*ps)[k] = v
		}
	}
}

func (ps *Parameters) Init() {
	if *ps == nil {
		*ps = make(Parameters)
	} else {
		ps.Clear()
	}
}

func (Parameters) ParseKeyValue(param string) (key, value string, err error) {
	param = strings.TrimSpace(param)
	if len(param) == 0 {
		return "", "", NewSyntaxError(ParamsSyntaxError, nil, param)
	}
	i := strings.IndexByte(param, '=')
	if i == 0 {
		return "", "", NewSyntaxError(ParamsSyntaxError, nil, param)
	} else if i == -1 {
		key, err = unescape(param, escapeEncodeQueryComponent)
		if err != nil {
			return "", "", NewValueError(ParamsNameError, err, "")
		}
	} else {
		key, err = unescape(param[:i], escapeEncodeQueryComponent)
		if err != nil {
			return "", "", NewValueError(ParamsNameError, err, "")
		}
		value, err = unescape(param[i+1:], escapeEncodeQueryComponent)
		if err != nil {
			return "", "", NewValueError(ParamsValueError, err, key)
		}
	}
	return
}

func (ps *Parameters) Parse(params string) error {
	paramsArr := strings.Split(params, ";")
	ps.Init()
	for _, param := range paramsArr {
		key, value, err := Parameters.ParseKeyValue(nil, param)
		if err != nil {
			return err
		}
		if _, has := (*ps)[key]; has {
			return NewSyntaxError(ParamsNameRepeatError, nil, key)
		}
		(*ps)[key] = value
	}
	return nil
}

func (ps Parameters) Equal(other Parameters) bool {
	var l, ol int
	if ps != nil {
		l = len(ps)
	}
	if other != nil {
		ol = len(other)
	}
	if l != ol {
		return false
	} else if l == 0 {
		return true
	}
	for k, v := range ps {
		if ov, has := other[k]; has {
			if v != ov {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

func (ps Parameters) EqualIgnoreNotContain(other Parameters) bool {
	if ps == nil || len(ps) == 0 {
		return true
	}
	if other == nil || len(other) == 0 {
		return true
	}
	for k, v := range ps {
		if ov, has := other[k]; has {
			if v != ov {
				return false
			}
		}
	}
	return true
}

func (ps Parameters) GuessLength() int {
	var l int
	if ps != nil {
		for k, v := range ps {
			l += len(k) + 1
			if len(v) > 0 {
				l += escapeRequire(v, escapeEncodeQueryComponent) + 1
			}
		}
	}
	return l
}

func (ps Parameters) Write(sb *bytes.Buffer) {
	if ps != nil {
		for k, v := range ps {
			sb.WriteByte(';')
			sb.WriteString(k)
			if len(v) > 0 {
				sb.WriteString("=")
				sb.WriteString(escape(v, escapeEncodeQueryComponent))
			}
		}
	}
}

func (ps Parameters) Clear() {
	if ps != nil {
		for key := range ps {
			delete(ps, key)
		}
	}
}

func (ps Parameters) String() string {
	sb := bytes.NewBuffer(make([]byte, 0, ps.GuessLength()))
	ps.Write(sb)
	return uns.BytesToString(sb.Bytes())
}
