package model

func clone[T any](obj *T) *T {
	if obj == nil {
		return nil
	}
	n := new(T)
	*n = *obj
	return obj
}
