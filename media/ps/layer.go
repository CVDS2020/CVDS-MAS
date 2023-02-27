package ps

type Layer struct {
	PSH PackHeader
	SYS SystemHeader
	PSM PSM
	PES []PES
}

func (l *Layer) Clear() *Layer {
	l.PSH.Clear()
	l.SYS.Clear()
	l.PSM.Clear()
	l.PES = l.PES[:0]
	return l
}

func (l *Layer) ExtendPES() {
	if len(l.PES) < cap(l.PES) {
		l.PES = l.PES[:len(l.PES)+1]
	} else if cap(l.PES) < 1024 {
		l.PES = make([]PES, cap(l.PES)*2+1)
	} else {
		l.PES = make([]PES, cap(l.PES)+cap(l.PES)>>2)
	}
}
