package errors

import "fmt"

type Exist struct {
	Target string
}

func NewExist(target string) *Exist {
	return &Exist{Target: target}
}

func (e *Exist) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s已经存在", e.Target)
}
