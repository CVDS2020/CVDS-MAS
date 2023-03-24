package ps

import "unsafe"

type StartCodeFinder struct {
	buffer [8]byte
	buffed int
}

func (f *StartCodeFinder) Reset() {
	f.buffed = 0
}

func (f *StartCodeFinder) Find(data []byte) (startCode [4]byte, remain []byte, ok bool) {
	if f.buffed+len(data) < 4 {
		f.buffed += copy(f.buffer[f.buffed:], data)
		return
	}

	p := data
	var index int
	for f.buffed+len(p) >= 8 {
		var startCode64 uint64
		buffed := f.buffed
		if buffed > 0 {
			f.buffed += copy(f.buffer[buffed:], p[:8-buffed])
			startCode64 = *((*uint64)(unsafe.Pointer(&f.buffer[0])))
			f.buffed = 0
			p = p[4-buffed:]
		} else {
			startCode64 = *((*uint64)(unsafe.Pointer(&p[0])))
			p = p[4:]
		}

		switch {
		case startCode64&0x00ffffff == 0x00010000:
			startCode[2], startCode[3] = 1, byte(startCode64>>24)
			index += 4 - buffed
		case startCode64&0x00ffffff00 == 0x0001000000:
			startCode[2], startCode[3] = 1, byte(startCode64>>32)
			index += 5 - buffed
		case startCode64&0x00ffffff0000 == 0x000100000000:
			startCode[2], startCode[3] = 1, byte(startCode64>>40)
			index += 6 - buffed
		case startCode64&0x00ffffff000000 == 0x00010000000000:
			startCode[2], startCode[3] = 1, byte(startCode64>>48)
			index += 7 - buffed
		case startCode64&0x00ffffff00000000 == 0x0001000000000000:
			startCode[2], startCode[3] = 1, byte(startCode64>>56)
			index += 8 - buffed
		default:
			index += 4 - buffed
			continue
		}
		remain, ok = data[index:], true
		return
	}

	if len(p) > 0 {
		buffed := f.buffed
		if f.buffed += copy(f.buffer[f.buffed:], p); f.buffed < 4 {
			return
		}
		for i := 0; i < f.buffed-3; i++ {
			startCode32 := *((*uint32)(unsafe.Pointer(&f.buffer[i])))
			if startCode32&0x00ffffff == 0x00010000 {
				startCode[2], startCode[3] = 1, byte(startCode32>>24)
				remain, ok = data[index+i+4-buffed:], true
				f.buffed = 0
				return
			}
		}
		copy(f.buffer[:], f.buffer[f.buffed-3:])
		f.buffed = 3
	}
	return
}
