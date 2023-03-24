package parser

import (
	"bytes"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/uns"
)

const DefaultMaxLineSize = 2048

type LineParser struct {
	MaxLineSize int
	cache       [][]byte
	cacheSize   int
	skip        bool
	line        string
}

func (lp *LineParser) reset() {
	lp.cache = lp.cache[:0]
	lp.cacheSize = 0
}

func (lp *LineParser) Parse(data []byte) (ok bool, remain []byte, err error) {
	if lp.skip {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
			lp.skip = false
		} else {
			return false, nil, nil
		}
	}
	if len(data) == 0 {
		return
	}
	if lp.MaxLineSize <= 0 {
		lp.MaxLineSize = DefaultMaxLineSize
	}
	limit := lp.MaxLineSize - lp.cacheSize
	if i := bytes.IndexByte(data, '\n'); i >= 0 && i <= limit {
		if i > 0 {
			var chunk []byte
			if data[i-1] == '\r' {
				// 换行符之前包含一个回车符
				chunk = data[:i-1]
			} else {
				// 换行符之前不包含回车符
				chunk = data[:i]
			}
			if len(chunk) > 0 {
				lp.cache = append(lp.cache, chunk)
				lp.cacheSize += len(chunk)
			}
		} else if len(lp.cache) > 0 {
			last := lp.cache[len(lp.cache)-1]
			if last[len(last)-1] == '\r' {
				// 上一次解析的数据中最后一个字符为回车符，需要去除掉
				if last = last[:len(last)-1]; len(last) == 0 {
					lp.cache = lp.cache[:len(lp.cache)-1]
				} else {
					lp.cache[len(lp.cache)-1] = last
				}
				lp.cacheSize--
			}
		}
		switch len(lp.cache) {
		case 0:
			lp.line = ""
		case 1:
			lp.line = uns.BytesToString(lp.cache[0])
		default:
			lp.line = uns.BytesToString(bytes.Join(lp.cache, nil))
		}
		lp.reset()
		return true, data[i+1:], nil
	} else if (i >= 0 && i > limit) || (i < 0 && len(data) >= limit) {
		// 不管是否找到了换行符，如果此时解析行的长度超过了限制，则返回错误
		var parsedLineSize int
		if i >= 0 {
			parsedLineSize = lp.cacheSize + i + 1
			remain = data[i+1:]
		} else {
			parsedLineSize = lp.cacheSize + len(data)
			lp.skip = true
		}
		lp.reset()
		return false, remain, errors.NewSizeOutOfRange("单行数据", 0, int64(lp.MaxLineSize), int64(parsedLineSize), false)
	} else {
		// 没找到换行符，并且此时解析行的长度没超过限制，将数据添加到缓存
		lp.cache = append(lp.cache, data)
		lp.cacheSize += len(data)
	}
	return false, nil, nil
}

func (lp *LineParser) ParseP(data []byte, remainP *[]byte) (ok bool, err error) {
	ok, *remainP, err = lp.Parse(data)
	return
}

func (lp *LineParser) Line() string {
	return lp.line
}

func (lp *LineParser) Reset() {
	lp.reset()
	lp.skip = false
	lp.line = ""
}
