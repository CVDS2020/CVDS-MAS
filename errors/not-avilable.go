package errors

import "fmt"

type NotAvailableError struct {
	Target string
}

func (e *NotAvailableError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s不可用", e.Target)
}
