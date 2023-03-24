package ps

import "gitee.com/sy_183/common/pool"

type Frame struct {
	psh *PSH
	sys *SYS
	psm *PSM
	pes []*PES

	relations pool.Relations

	pshPool pool.Pool[*PSH]
	sysPool pool.Pool[*SYS]
	psmPool pool.Pool[*PSM]
	pesPool pool.Pool[*PES]
	pool    pool.Pool[*Frame]
}

func (f *Frame) PSH() *PSH {
	return f.psh
}

func (f *Frame) SYS() *SYS {
	return f.sys
}

func (f *Frame) PSM() *PSM {
	return f.psm
}

func (f *Frame) PES() []*PES {
	return f.pes
}

func (f *Frame) AddRelation(relation pool.Reference) {
	f.relations.AddRelation(relation)
}

func (f *Frame) Clear() {
	f.relations.Clear()
	if f.psh != nil && f.pshPool != nil {
		f.psh.Clear()
		f.pshPool.Put(f.psh)
		f.psh = nil
	}
	if f.sys != nil && f.sysPool != nil {
		f.sys.Clear()
		f.sysPool.Put(f.sys)
		f.sys = nil
	}
	if f.psm != nil && f.psmPool != nil {
		f.psm.Clear()
		f.psmPool.Put(f.psm)
		f.psm = nil
	}
	for _, pes := range f.pes {
		if f.pesPool != nil {
			pes.Clear()
			f.pesPool.Put(pes)
		}
	}
	f.pes = f.pes[:0]
}

func (f *Frame) Release() bool {
	if f.relations.Release() {
		f.Clear()
		if f.pool != nil {
			f.pool.Put(f)
		}
		return true
	}
	return false
}

func (f *Frame) AddRef() {
	f.relations.AddRef()
}

func (f *Frame) Use() *Frame {
	f.AddRef()
	return f
}
