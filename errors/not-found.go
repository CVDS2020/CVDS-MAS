package errors

import "fmt"

type NotFound struct {
	Target string
}

func NewNotFound(target string) *NotFound {
	return &NotFound{Target: target}
}

func (e *NotFound) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s未找到", e.Target)
}
