package rtsp

type Option interface {
	apply(target any) any
}

type optionFunc func(target any) any

func (f optionFunc) apply(target any) any {
	return f(target)
}
