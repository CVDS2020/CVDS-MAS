package errors

type InvalidOperation struct {
	Operation string
}

func (e *InvalidOperation) Error() string {
	if e == nil {
		return "<nil>"
	}
	return "非法的操作: " + e.Operation
}
