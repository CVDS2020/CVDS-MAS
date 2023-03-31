package ps

import "io"

type Pack interface {
	StartCode() [4]byte

	Size() int

	io.WriterTo

	Clear()
}
