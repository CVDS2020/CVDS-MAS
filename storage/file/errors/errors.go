package errors

type Err struct {
	Err error
}

func (e *Err) Error() string {
	return e.Err.Error()
}

type OperationError struct {
	Operation string
}

func (o *OperationError) Error() string {
	if o == nil {
		return "<nil>"
	}
	return "invalid operation: " + o.Operation
}

type OpenError struct{ Err }
type WriteError struct{ Err }
type GetInfoError struct{ Err }
type SyncError struct{ Err }
type SeekError struct{ Err }
type CloseError struct{ Err }
type RemoveError struct{ Err }
type FileOpeningError struct{ Err }
