package ps

import (
	"errors"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/cvds-mas/media"
	"math"
)

type StreamInfo struct {
	media.BaseStreamInfo
	streamId   uint8
	streamType uint8
}

func NewPsStreamInfo(mediaType *media.MediaType, parent media.StreamInfo, streamId uint8, streamType uint8) *StreamInfo {
	i := new(StreamInfo)
	i.BaseStreamInfo.Init(mediaType, parent)
	i.streamId = streamId
	i.streamType = streamType
	return i
}

func (i *StreamInfo) StreamId() uint8 {
	return i.streamId
}

func (i *StreamInfo) StreamType() uint8 {
	return i.streamType
}

func (i *StreamInfo) Equal(other media.StreamInfo) bool {
	if i == nil && other == nil {
		return true
	} else if i == nil || other == nil {
		return false
	}
	o, ok := other.(*StreamInfo)
	if !ok || o == nil {
		return false
	}
	return i.MediaType().ID == o.MediaType().ID && i.streamId == o.streamId && i.streamType == o.streamType
}

type StreamInfos struct {
	streamInfos  []*StreamInfo
	streamIdMap  map[uint8]*StreamInfo
	mediaTypeMap map[uint32]*StreamInfo
}

func NewPsStreamInfos(streamInfos []*StreamInfo) *StreamInfos {
	is := &StreamInfos{
		streamInfos: streamInfos,
	}
	if len(streamInfos) >= 8 {
		is.streamIdMap = make(map[uint8]*StreamInfo)
		is.mediaTypeMap = make(map[uint32]*StreamInfo)
		for _, info := range streamInfos {
			is.streamIdMap[info.streamId] = info
			is.mediaTypeMap[info.MediaType().ID] = info
		}
	}
	return is
}

func (is *StreamInfos) Len() int {
	return len(is.streamInfos)
}

func (is *StreamInfos) StreamInfos() []*StreamInfo {
	return is.streamInfos
}

func (is *StreamInfos) GetStreamInfoById(id uint8) *StreamInfo {
	if is.streamIdMap != nil {
		return is.streamIdMap[id]
	}
	for _, info := range is.streamInfos {
		if info.streamId == id {
			return info
		}
	}
	return nil
}

func (is *StreamInfos) GetStreamInfoByMediaType(typeId uint32) *StreamInfo {
	if is.mediaTypeMap != nil {
		return is.mediaTypeMap[typeId]
	}
	for _, info := range is.streamInfos {
		if info.MediaType().ID == typeId {
			return info
		}
	}
	return nil
}

type FrameParser struct {
	parser Parser

	pshPool   pool.Pool[*PSH]
	sysPool   pool.Pool[*SYS]
	psmPool   pool.Pool[*PSM]
	pesPool   pool.Pool[*PES]
	framePool pool.Pool[*Frame]

	parentStreamInfo media.StreamInfo
	streamInfos      *StreamInfos

	curFrame *Frame
	frame    *Frame
}

func newPSH(p *pool.SyncPool[*PSH]) *PSH { return new(PSH) }
func newSYS(p *pool.SyncPool[*SYS]) *SYS { return new(SYS) }
func newPSM(p *pool.SyncPool[*PSM]) *PSM { return new(PSM) }
func newPES(p *pool.SyncPool[*PES]) *PES { return new(PES) }

func NewFrameParser() *FrameParser {
	p := &FrameParser{
		pshPool: pool.NewSyncPool[*PSH](newPSH, pool.WithLimit(math.MaxInt64)),
		sysPool: pool.NewSyncPool[*SYS](newSYS, pool.WithLimit(math.MaxInt64)),
		psmPool: pool.NewSyncPool[*PSM](newPSM, pool.WithLimit(math.MaxInt64)),
		pesPool: pool.NewSyncPool[*PES](newPES, pool.WithLimit(math.MaxInt64)),
	}
	p.framePool = pool.NewSyncPool[*Frame](p.newFrame, pool.WithLimit(math.MaxInt64))
	p.parser.SetPSH(p.pshPool.Get())
	p.parser.SetSYS(p.sysPool.Get())
	p.parser.SetPSM(p.psmPool.Get())
	p.parser.SetPES(p.pesPool.Get())
	return p
}

func (p *FrameParser) newFrame(*pool.SyncPool[*Frame]) *Frame {
	return &Frame{
		pshPool: p.pshPool,
		sysPool: p.sysPool,
		psmPool: p.psmPool,
		pesPool: p.pesPool,
		pool:    p.framePool,
	}
}

func (p *FrameParser) ParentStreamInfo() media.StreamInfo {
	return p.parentStreamInfo
}

func (p *FrameParser) SetParentStreamInfo(streamInfo media.StreamInfo) {
	p.parentStreamInfo = streamInfo
}

func (p *FrameParser) StreamInfos() *StreamInfos {
	return p.streamInfos
}

func (p *FrameParser) complete() {
	if p.curFrame.psm != nil {
		var streamInfos []*StreamInfo
		var streamIdSet [32]byte
		for _, info := range p.curFrame.psm.ElementaryStreamInfos() {
			if mediaType := StreamTypeToMediaType(info.Type()); mediaType != nil {
				streamId := info.Id()
				if offset, mask := streamId>>3, uint8(1<<(streamId|0b111)); streamIdSet[offset]&mask == 0 {
					streamInfos = append(streamInfos, NewPsStreamInfo(mediaType, p.parentStreamInfo, info.Id(), info.Type()))
					streamIdSet[offset] |= mask
				}
			}
		}
		equal := true
		if p.streamInfos == nil {
			equal = false
		} else {
			if p.streamInfos.Len() != len(streamInfos) {
				equal = false
			} else {
				for _, info := range streamInfos {
					if i := p.streamInfos.GetStreamInfoById(info.streamId); i != nil {
						if !i.Equal(info) {
							equal = false
							break
						}
					}
				}
			}
		}
		if !equal {
			p.streamInfos = NewPsStreamInfos(streamInfos)
		}
	}
	if p.frame != nil {
		// 覆盖之前未拿走的PS帧
		p.frame.Release()
		p.frame = nil
	}
	p.frame = p.curFrame
	p.curFrame = nil
	return
}

func (p *FrameParser) getCurFrame() *Frame {
	if p.curFrame == nil {
		p.curFrame = p.framePool.Get()
		if p.curFrame == nil {
			return nil
		}
		p.curFrame.Use()
	}
	return p.curFrame
}

func (p *FrameParser) resetPSH() bool {
	psh := p.pshPool.Get()
	if psh == nil {
		return false
	}
	p.parser.SetPSH(psh)
	return true
}

func (p *FrameParser) resetSYS() bool {
	sys := p.sysPool.Get()
	if sys == nil {
		return false
	}
	p.parser.SetSYS(sys)
	return true
}

func (p *FrameParser) resetPSM() bool {
	psm := p.psmPool.Get()
	if psm == nil {
		return false
	}
	p.parser.SetPSM(psm)
	return true
}

func (p *FrameParser) resetPES() bool {
	pes := p.pesPool.Get()
	if pes == nil {
		return false
	}
	p.parser.SetPES(pes)
	return true
}

// 解析PS流数据块，返回值ok为真时说明解析完成，可以通过 Get 方法或 Take 方法获取到解析完成的PS帧，
// 返回值remain为解析完成后剩余的数据。此方法需要注意：当返回包含错误时，此时返回值ok有可能为真
func (p *FrameParser) Parse(data []byte) (ok bool, remain []byte, err error) {
	defer func() {
		if err != nil {
			if p.curFrame != nil {
				p.curFrame.Release()
			}
		}
	}()

	remain = data
	for len(remain) > 0 {
		if ok, err := p.parser.ParseP(remain, &remain); err != nil {
			return false, remain, err
		} else if ok {
			switch pack := p.parser.Pack().(type) {
			case *PSH:
				curFrame := p.getCurFrame()
				if curFrame == nil {
					return true, remain, errors.New("申请PS帧失败")
				}
				if curFrame.psh == nil {
					curFrame.psh = pack
					if !p.resetPSH() {
						return false, remain, errors.New("申请PS.PSH失败")
					}
				} else {
					p.complete()
					curFrame = p.getCurFrame()
					if curFrame == nil {
						return true, remain, errors.New("申请PS帧失败")
					}
					curFrame.psh = pack
					if !p.resetPSH() {
						return true, remain, errors.New("申请PS.PSH失败")
					}
					return true, remain, nil
				}
			case *SYS:
				if curFrame := p.curFrame; curFrame != nil {
					if curFrame.sys == nil {
						curFrame.sys = pack
						if !p.resetSYS() {
							return false, remain, errors.New("申请PS.SYS失败")
						}
					} else {
						pack.Clear()
						return false, remain, errors.New("重复的PS.SYS")
					}
				} else {
					pack.Clear()
				}
			case *PSM:
				if curFrame := p.curFrame; curFrame != nil {
					if curFrame.psm == nil {
						curFrame.psm = pack
						if !p.resetPSM() {
							return false, remain, errors.New("申请PS.PSM失败")
						}
					} else {
						pack.Clear()
						return false, remain, errors.New("重复的PS.PSM")
					}
				} else {
					pack.Clear()
				}
			case *PES:
				if curFrame := p.curFrame; curFrame != nil {
					curFrame.pes = append(curFrame.pes, pack)
					if !p.resetPES() {
						return false, remain, errors.New("申请PS.PES失败")
					}
				} else {
					pack.Clear()
				}
			}
		}
	}
	return
}

func (p *FrameParser) ParseP(data []byte, remainP *[]byte) (ok bool, err error) {
	ok, *remainP, err = p.Parse(data)
	return
}

// Complete 方法指明正在解析的PS帧解析完成，在不调用此方法的情况下，只有当解析到下一个PSH的时候才会将
// 当前正在解析的PS帧放入解析完成的PS缓存帧中，此方法一般在解析到PS流结尾时(不会再出现新的PSH时)调用
func (p *FrameParser) Complete() bool {
	if p.curFrame != nil {
		p.complete()
		return true
	}
	return false
}

// Reset 方法重置解析器的状态，当前正在解析的PS帧和解析完成的PS缓存帧也会被释放，但是不会清空流信息
func (p *FrameParser) Reset() {
	p.Complete()
	if p.frame != nil {
		p.frame.Release()
		p.frame = nil
	}
	p.parser.Reset()
}

// Get 方法获取解析完成的PS帧，当执行 Parse 方法返回解析完成后，一定可以获取到非空的PS帧，并且此操作不
// 会清空解析器中缓存的PS帧，解析完成后可多次调用获取，并且每次获取的PS帧引用计数加一
func (p *FrameParser) Get() *Frame {
	if p.frame != nil {
		return p.frame.Use()
	}
	return nil
}

// Take 方法拿走解析完成的PS帧，当执行 Parse 方法返回解析完成后，一定可以拿到到非空的PS帧，此操作会清
// 空解析器的缓存帧，所以解析完成后调用一次即可，拿到的PS帧不会增加引用计数
func (p *FrameParser) Take() (frame *Frame) {
	frame = p.frame
	p.frame = nil
	return
}

// Free 方法释放解析器中的所有从对象池申请的资源，包括当前解析的PS包、当前解析的PS帧和解析完成的PS缓存
// 帧，释放完成后的解析器不可以再解析任何数据，但是PS流信息不会被清空
func (p *FrameParser) Free() {
	p.Reset()
	if old := p.parser.SetPSH(nil); old != nil {
		old.Clear()
		p.pshPool.Put(old)
	}
	if old := p.parser.SetSYS(nil); old != nil {
		old.Clear()
		p.sysPool.Put(old)
	}
	if old := p.parser.SetPSM(nil); old != nil {
		old.Clear()
		p.psmPool.Put(old)
	}
	if old := p.parser.SetPES(nil); old != nil {
		old.Clear()
		p.pesPool.Put(old)
	}
}
