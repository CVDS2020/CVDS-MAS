package sip

import (
	"errors"
	"strconv"
	"strings"
)

type URIHeader map[string][]string

func (h *URIHeader) CopyFrom(other URIHeader) {
	h.Clear()
	if h != nil && len(other) > 0 {
		if *h == nil {
			*h = make(URIHeader)
		}
		for k, vs := range other {
			for _, v := range vs {
				(*h)[k] = append((*h)[k], v)
			}
		}
	}
}

func (h *URIHeader) Parse(header string) error {
	headerArr := strings.Split(header, "&")
	if *h == nil {
		*h = make(URIHeader)
	} else {
		h.Clear()
	}
	for _, field := range headerArr {
		field = strings.TrimSpace(field)
		if len(field) == 0 {
			continue
		}
		var key, value string
		var err error
		i := strings.IndexByte(field, '=')
		if i == 0 {
			return NewSyntaxError(URISyntaxError, nil, header)
		} else if i == -1 {
			key, err = unescape(field, escapeEncodeQueryComponent)
			if err != nil {
				return NewValueError(URIValueError, err, "header-name")
			}
		} else {
			key, err = unescape(field[:i], escapeEncodeQueryComponent)
			if err != nil {
				return NewValueError(URIValueError, err, "header-name")
			}
			value, err = unescape(field[i+1:], escapeEncodeQueryComponent)
			if err != nil {
				return NewValueError(URIValueError, err, key)
			}
		}
		(*h)[key] = append((*h)[key], value)
	}
	return nil
}

func (h URIHeader) Equal(other URIHeader) bool {
	var l, ol int
	if h != nil {
		l = len(h)
	}
	if other != nil {
		ol = len(other)
	}
	if l != ol {
		return false
	} else if l == 0 {
		return true
	}
	for k, vs := range h {
		if ovs, has := other[k]; has {
			if len(vs) == len(ovs) {
				for i, v := range vs {
					if v != ovs[i] {
						return false
					}
				}
			} else {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

func (h URIHeader) Clear() {
	if h != nil {
		for key := range h {
			delete(h, key)
		}
	}
}

var URISyntaxError = ErrorType{Type: iotaET(), Description: "sip uri syntax error"}
var URIValueError = ErrorType{Type: iotaET(), Description: "sip uri value error"}

type URI struct {
	Scheme         string
	User           string
	Password       string
	Host           string
	Port           int
	Headers        URIHeader
	TransportParam string
	TTLParam       int
	MethodParam    string
	UserParam      string
	MAddrParam     string
	LrParam        string
	Params         Parameters
}

func parseNumber(np *int, value string, limit int) error {
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed > uint64(limit) {
		return err
	}
	*np = int(parsed)
	return nil
}

func ParseHost(host string) (string, error) {
	if strings.HasPrefix(host, "[") {
		// ParseURL an IP-Literal in RFC 3986 and RFC 6874.
		// E.g., "[fe80::1]", "[fe80::1%25en0]", "[fe80::1]:80".
		if host[len(host)-1] != ']' {
			return "", NewHostError(HostFormatError, nil, host)
		}
		host = host[1 : len(host)-1]
		i := strings.Index(host, "%25")
		if i > 0 {
			ipv6, err := unescape(host[:i], escapeEncodeHost)
			if err != nil {
				return "", NewHostError(HostFormatError, err, host)
			}
			zone, err := unescape(host[i:], escapeEncodeZone)
			if err != nil {
				return "", NewHostError(HostFormatError, err, host)
			}
			return ipv6 + zone, nil
		}
	}
	return host, nil
}

func ParsePort(port string) (int, error) {
	parsed, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0, errors.New("")
	}
	return int(parsed), nil
}

func ParseHostPort(hostPort string) (host string, port int, err error) {
	i := strings.LastIndexByte(hostPort, ':')
	if i == 0 {
		return "", 0, errors.New("")
	}
	port = -1
	if i != -1 {
		port, err = ParsePort(hostPort[i+1:])
		if err != nil {
			return
		}
		hostPort = hostPort[:i]
	}
	host, err = ParseHost(hostPort)
	if err != nil {
		return
	}
	return
}

func (u *URI) parseScheme(rawURI string) (rest string, err error) {
	i := strings.IndexByte(rawURI, ':')
	if i <= 0 {
		return "", NewSyntaxError(URISyntaxError, nil, rawURI)
	}
	u.Scheme = rawURI[:i]
	return rawURI[i+1:], nil
}

func (u *URI) parseUserInfo(userInfo string) error {
	i := strings.IndexByte(userInfo, ':')
	if i == 0 {
		return NewSyntaxError(URISyntaxError, nil, userInfo)
	}
	if i == -1 {
		username, err := unescape(userInfo, escapeEncodeUserPassword)
		if err != nil {
			return NewValueError(URIValueError, err, "username")
		}
		u.User = username
		return nil
	}
	username, err := unescape(userInfo[:i], escapeEncodeUserPassword)
	if err != nil {
		return NewValueError(URIValueError, err, "username")
	}
	password, err := unescape(userInfo[i+1:], escapeEncodeUserPassword)
	if err != nil {
		return NewValueError(URIValueError, err, "password")
	}
	u.User, u.Password = username, password
	return nil
}

func (u *URI) parseHostPort(hostPort string) error {
	host, port, err := ParseHostPort(hostPort)
	if err != nil {
		return NewValueError(URIValueError, err, "host:port")
	}
	u.Host = host
	if port != -1 {
		u.Port = port
	}
	return nil
}

func (u *URI) parseBase(base string) (rest string, err error) {
	i := strings.IndexAny(base, ";?")
	if i == 0 {
		return "", NewSyntaxError(URISyntaxError, nil, base)
	} else if i != -1 {
		base, rest = strings.TrimSpace(base[:i]), base[i:]
	}

	i = strings.IndexByte(base, '@')
	if i == 0 {
		return "", NewSyntaxError(URISyntaxError, nil, base)
	}
	if i != -1 {
		userInfo := base[:i]
		err = u.parseUserInfo(userInfo)
		if err != nil {
			return "", err
		}
		base = base[i+1:]
	}

	err = u.parseHostPort(base)
	if err != nil {
		return "", err
	}
	return
}

func (u *URI) parseParams(params string) (rest string, err error) {
	i := strings.IndexByte(params, '?')
	if i == 0 {
		return "", NewSyntaxError(URISyntaxError, nil, params)
	} else if i != -1 {
		params, rest = params[:i], params[i:]
	}
	paramsArr := strings.Split(params, ";")
	u.Params.Init()
	for _, param := range paramsArr {
		key, value, err := Parameters.ParseKeyValue(nil, param)
		if err != nil {
			return "", err
		}
		switch strings.ToLower(key) {
		case "user":
			if value == "" {
				return "", NewValueError(ParamsValueError, nil, key)
			}
			u.UserParam = value
		case "method":
			if value == "" {
				return "", NewValueError(ParamsValueError, nil, key)
			}
			u.MethodParam = value
		case "transport":
			if value == "" {
				return "", NewValueError(ParamsValueError, nil, key)
			}
			u.TransportParam = value
		case "ttl":
			if value == "" {
				return "", NewValueError(ParamsValueError, nil, key)
			}
			err = parseNumber(&u.TTLParam, value, 255)
			if err != nil {
				return "", NewValueError(ParamsValueError, err, key)
			}
		case "lr":
			if value != "" {
				return "", NewValueError(ParamsValueError, nil, key)
			}
		case "maddr":
			if value == "" {
				return "", NewValueError(ParamsValueError, nil, key)
			}
			u.MAddrParam = value
		default:
			if _, has := u.Params[key]; has {
				return "", NewSyntaxError(ParamsNameRepeatError, nil, key)
			}
			u.Params[key] = value
		}
	}
	return
}

func (u *URI) parseHeader(header string) error {
	return u.Headers.Parse(header)
}

func (u *URI) Parse(raw string) error {
	rest, err := u.parseScheme(raw)
	if err != nil {
		return err
	}

	rest, err = u.parseBase(rest)
	if err != nil {
		return err
	}

	if len(rest) == 0 {
		return nil
	} else if rest[0] == ';' {
		rest, err = u.parseParams(rest[1:])
		if err != nil {
			return err
		}
	}

	if len(rest) == 0 {
		return nil
	}
	return u.parseHeader(rest[1:])
}
