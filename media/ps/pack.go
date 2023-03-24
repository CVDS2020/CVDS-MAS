package ps

import "io"

type Pack interface {
	StartCode() [4]byte

	Size() uint

	io.WriterTo

	Clear()
}
