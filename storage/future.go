package storage

type Future[RES any] struct {
	Callback func(res RES)
	Channel  chan RES
}

func (f *Future[RES]) Response(res RES) {
	if f.Callback != nil {
		f.Callback(res)
	}
	if f.Channel != nil {
		f.Channel <- res
	}
}
