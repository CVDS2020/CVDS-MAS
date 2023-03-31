package channel

import (
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	timerPkg "gitee.com/sy_183/common/timer"
	"gitee.com/sy_183/common/utils"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/media/h264"
	"gitee.com/sy_183/cvds-mas/media/ps"
	"gitee.com/sy_183/cvds-mas/storage"
	rtpFramePkg "gitee.com/sy_183/rtp/frame"
	rtpServer "gitee.com/sy_183/rtp/server"
	"io"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type (
	HistoryRtpPusherEOFCallback    func(pusher *HistoryRtpPusher, channel *Channel)
	HistoryRtpPusherClosedCallback func(pusher *HistoryRtpPusher, channel *Channel)
)

type readResponse struct {
	request storage.Index
	data    []byte
	err     error
}

type blockGetter struct {
	lifecycle.Lifecycle

	storageChannel storage.Channel

	indexGetter storage.IndexGetter

	curBlock     []byte
	curBlockSeq  uint64
	nextBlockSeq uint64
	blockBuf     []byte

	eof         bool
	getIndexErr error
	seekFlag    bool

	readSession      storage.ReadSession
	readRequestChan  chan storage.Index
	readResponseChan chan *readResponse

	blockReading bool
}

func newBlockGetter(channel storage.Channel, indexGetter storage.IndexGetter, readSession storage.ReadSession) *blockGetter {
	getter := &blockGetter{
		storageChannel: channel,
		indexGetter:    indexGetter,

		curBlock: make([]byte, channel.WriteBufferSize()),
		blockBuf: make([]byte, channel.WriteBufferSize()),

		readSession:      readSession,
		readRequestChan:  make(chan storage.Index, 1),
		readResponseChan: make(chan *readResponse, 1),
	}
	getter.Lifecycle = lifecycle.NewWithInterruptedRun(nil, getter.blockReaderRun)
	return getter
}

func (g *blockGetter) blockReaderRun(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	for {
		select {
		case index := <-g.readRequestChan:
			var buf []byte
			if size := index.Size(); size > uint64(cap(g.blockBuf)) {
				buf = make([]byte, size)
			} else {
				buf = g.blockBuf[:size]
			}
			n, err := g.readSession.Read(buf, index)
			if err != nil {
				g.readResponseChan <- &readResponse{request: index, err: err}
				continue
			}
			g.readResponseChan <- &readResponse{request: index, data: buf[:n]}
		case <-interrupter:
			return nil
		}
	}
}

// readNext 获取下一个索引并请求读取对应的 block，如果获取到的索引为当前 block 的索引
// (由于 seek 改变了位置)，则直接返回当前的 block 并再次获取下一个索引并请求读取对应的
// block，此时下一个索引一定不与当前 block 的索引相同
func (g *blockGetter) readNext() (storage.Index, []byte, error) {
	index, err := g.indexGetter.Get()
	if err != nil {
		g.getIndexErr = err
		g.blockReading = false
		return nil, nil, err
	}
	if g.curBlockSeq == index.Seq() {
		// 由于 seek 的原因，获取到的索引与当前 block 的索引一致，提前获取下一个索引
		// 并请求读取对应的 block
		g.readNext()
		return index, g.curBlock, nil
	}
	g.getIndexErr = nil
	g.nextBlockSeq = index.Seq()
	g.readRequestChan <- index
	g.blockReading = true
	return index, nil, nil
}

// getAfterRead 从上次请求的响应中获取 block，并预先请求读取下一个索引对应的 block
func (g *blockGetter) getAfterRead() ([]byte, error) {
	res := <-g.readResponseChan
	if res.err != nil {
		g.readNext()
		return nil, res.err
	}
	g.curBlockSeq = res.request.Seq()
	g.curBlock, g.blockBuf = res.data, g.curBlock[:cap(g.curBlock)]
	g.readNext()
	return g.curBlock, nil
}

// getAfterReadFrom 从上次请求的响应中获取 block，判断上次请求的 block 的索引是否是
// 本次期望的索引，如果是，则直接返回获取到的 block，否则
func (g *blockGetter) getAfterReadFrom(index storage.Index) ([]byte, error) {
	res := <-g.readResponseChan
	if res.err != nil {
		if res.request.Seq() == index.Seq() {
			g.readNext()
			return nil, res.err
		} else {
			return g.get(index)
		}
	}
	g.curBlockSeq = res.request.Seq()
	g.curBlock, g.blockBuf = res.data, g.curBlock[:cap(g.curBlock)]
	if g.curBlockSeq == index.Seq() {
		g.readNext()
		return g.curBlock, nil
	}
	return g.get(index)
}

func (g *blockGetter) get(index storage.Index) ([]byte, error) {
	g.readRequestChan <- index
	g.blockReading = true
	return g.getAfterRead()
}

func (g *blockGetter) Get() ([]byte, error) {
	if g.eof {
		return nil, io.EOF
	}

	if g.seekFlag {
		g.seekFlag = false
		if index, block, err := g.readNext(); err != nil {
			// 获取索引时出现错误或读取到末尾
			if err == io.EOF {
				return nil, io.EOF
			}
			return nil, err
		} else if block != nil {
			// 由于 seek 的原因，获取到的索引与当前 block 的索引一致，直接返回
			return block, nil
		} else {
			return g.getAfterReadFrom(index)
		}
	} else {
		if g.blockReading {
			return g.getAfterRead()
		} else if g.getIndexErr != nil {
			// 上一次获取索引时出现错误或读取到末尾
			if g.getIndexErr == io.EOF {
				g.eof = true
				return nil, io.EOF
			}
			err := g.getIndexErr
			g.readNext()
			return nil, err
		} else {
			// 第一次获取 block
			g.readNext()
			if g.getIndexErr != nil {
				// 获取索引时出现错误或读取到末尾
				if g.getIndexErr == io.EOF {
					g.eof = true
					return nil, io.EOF
				}
				err := g.getIndexErr
				g.readNext()
				return nil, err
			}
			return g.getAfterRead()
		}
	}
}

func (g *blockGetter) Seek(t int64) {
	g.indexGetter.Seek(t)
	g.seekFlag = true
	g.eof = false
}

type HistoryRtpPusher struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle
	once   atomic.Bool

	id             uint64
	channel        *Channel
	storageChannel storage.Channel

	streamContextMap map[uint8]*streamContext

	psFrameParser *ps.FrameParser
	keepChooser   rtpServer.KeepChooser

	h264NaluParser  *h264.AnnexBInPrefixNALUParser
	h264RtpPackager *h264.RtpPackager

	outputRtpFramePool pool.Pool[*rtpFramePkg.DefaultFrame]

	streamPusherId    atomic.Uint64
	streamPusherCount atomic.Int64
	streamPushers     container.SyncMap[uint64, *RtpStreamPusher]
	streamPusherLock  sync.Mutex

	readSession storage.ReadSession
	indexGetter storage.IndexGetter
	blockGetter *blockGetter

	startTime int64
	endTime   int64

	timestamp time.Duration
	interval  time.Duration

	firstPush   bool
	sendTimer   *timerPkg.Timer
	sendSignal  chan struct{}
	pushSignal  chan struct{}
	pauseSignal chan struct{}
	seekSignal  chan int64
	scale       atomic.Uint64

	eofCallback     HistoryRtpPusherEOFCallback
	autoEOFCallback bool

	log.AtomicLogger
}

func newHistoryRtpPusher(id uint64, channel *Channel, startTime, endTime int64, eofCallback HistoryRtpPusherEOFCallback) (*HistoryRtpPusher, error) {
	if startTime == 0 {
		startTime = time.Now().Add(-time.Hour).UnixMilli()
	}
	if endTime == 0 {
		endTime = startTime + int64(time.Minute*10/time.Millisecond)
	}
	if endTime < startTime+1000 {
		endTime = startTime + 1000
	}
	storageChannel := channel.StorageChannel()
	if storageChannel == nil {
		return nil, StorageChannelNotSetupError
	}
	readSession, err := storageChannel.NewReadSession()
	if err != nil {
		return nil, err
	}

	p := &HistoryRtpPusher{
		id:             id,
		channel:        channel,
		storageChannel: storageChannel,

		streamContextMap: make(map[uint8]*streamContext),

		psFrameParser: ps.NewFrameParser(),
		keepChooser:   rtpServer.NewDefaultKeepChooser(3, 3, nil),

		outputRtpFramePool: pool.ProvideSyncPool[*rtpFramePkg.DefaultFrame](rtpFramePkg.NewDefaultFrame, pool.WithLimit(math.MaxInt64)),

		readSession: readSession,
		indexGetter: storage.NewCachedIndexGetter(storageChannel, startTime, endTime, 0, 0),

		startTime: startTime,
		endTime:   endTime,

		firstPush:   true,
		sendSignal:  make(chan struct{}, 1),
		pushSignal:  make(chan struct{}, 1),
		pauseSignal: make(chan struct{}, 1),
		seekSignal:  make(chan int64, 1),

		eofCallback:     eofCallback,
		autoEOFCallback: true,
	}

	p.blockGetter = newBlockGetter(storageChannel, p.indexGetter, p.readSession)
	p.sendTimer = timerPkg.NewTimer(p.sendSignal)
	p.SetScale(1)
	p.SetLogger(channel.Logger().Named(p.DisplayName()))
	p.runner = lifecycle.NewWithInterruptedRun(p.start, p.run)
	p.Lifecycle = p.runner
	return p, nil
}

func (p *HistoryRtpPusher) Id() uint64 {
	return p.id
}

func (p *HistoryRtpPusher) DisplayName() string {
	return p.channel.HistoryRtpPlayerDisplayName(p.id)
}

func (p *HistoryRtpPusher) StreamPushers() []*RtpStreamPusher {
	return p.streamPushers.Values()
}

func (p *HistoryRtpPusher) AddStream(transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (streamPusher *RtpStreamPusher, err error) {
	return lock.RLockGetDouble(p.runner, func() (streamPusher *RtpStreamPusher, err error) {
		if !p.runner.Running() {
			return nil, p.Logger().ErrorWith("历史音视频RTP推流通道添加失败", lifecycle.NewStateNotRunningError(p.DisplayName()))
		}
		streamId := p.streamPusherId.Add(1)
		streamPusher, err = newRtpStreamPusher(streamId, nil, p, transport, remoteIp, remotePort, ssrc, streamInfo)
		if err != nil {
			return nil, err
		}
		lock.LockDo(&p.streamPusherLock, func() {
			p.streamPushers.Store(streamId, streamPusher)
			p.streamPusherCount.Add(1)
		})
		return streamPusher, nil
	})
}

func (p *HistoryRtpPusher) RemoveStream(streamId uint64) error {
	streamPusher, closePusher := lock.LockGetDouble(&p.streamPusherLock, func() (*RtpStreamPusher, bool) {
		streamPlayer, _ := p.streamPushers.LoadAndDelete(streamId)
		if streamPlayer != nil {
			return streamPlayer, p.streamPusherCount.Add(-1) == 0
		}
		return nil, false
	})
	if streamPusher != nil {
		streamPusher.close()
	}
	if closePusher {
		p.Close(nil)
	}
	return nil
}

func (p *HistoryRtpPusher) getH264NaluParser() *h264.AnnexBInPrefixNALUParser {
	if p.h264NaluParser == nil {
		p.h264NaluParser = h264.NewAnnexBInPrefixNALUParser(0)
	}
	return p.h264NaluParser
}

func (p *HistoryRtpPusher) getH264RtpPackager() *h264.RtpPackager {
	if p.h264RtpPackager == nil {
		p.h264RtpPackager = h264.NewRtpPackager(0)
	}
	return p.h264RtpPackager
}

func (p *HistoryRtpPusher) getOutputRtpFrame(streamCtx *streamContext) rtpFramePkg.Frame {
	if streamCtx.outputRtpFrame == nil {
		streamCtx.outputRtpFrame = p.outputRtpFramePool.Get()
		if streamCtx.outputRtpFrame == nil {
			return nil
		}
		streamCtx.outputRtpFrame.AddRef()
	}
	return streamCtx.outputRtpFrame
}

func (p *HistoryRtpPusher) waitSend(interrupter chan struct{}) (interrupted bool, seek bool) {
	for {
		select {
		case <-p.sendSignal:
			// 到达了此帧应该发送的时刻
			p.sendTimer.After(p.interval)
			return false, false
		case <-p.pushSignal:
			if p.firstPush {
				p.sendTimer.Trigger()
				p.firstPush = false
			}
		case t := <-p.seekSignal:
			// 有 seek 请求，忽略此帧的发送
			p.blockGetter.Seek(t)
			return false, true
		case <-p.pauseSignal:
			// 有暂停的请求，等待恢复的信号或者 seek 信号
		waitPush:
			for {
				select {
				case <-p.pushSignal:
					if p.firstPush {
						p.sendTimer.Trigger()
						p.firstPush = false
					}
					break waitPush
				case <-p.pauseSignal:
				case t := <-p.seekSignal:
					p.blockGetter.Seek(t)
					return false, true
				case <-interrupter:
					return true, false
				}
			}
		case <-interrupter:
			return true, false
		}
	}
}

func (p *HistoryRtpPusher) waitClose(interrupter chan struct{}) (interrupted bool, seek bool) {
	for {
		select {
		case t := <-p.seekSignal:
			p.blockGetter.Seek(t)
			p.autoEOFCallback = true
			return false, true
		case <-p.pushSignal:
			if p.firstPush {
				p.sendTimer.Trigger()
				p.firstPush = false
			}
		case <-p.pauseSignal:
		case <-interrupter:
			return true, false
		}
	}
}

func (p *HistoryRtpPusher) handlePsFrame(psFrame *ps.Frame) (keep bool) {
	defer psFrame.Release()
	if len(psFrame.PES()) == 0 {
		// PS帧中不包含PES数据
		return true
	}

	psStreamInfos := p.psFrameParser.StreamInfos()
	if psStreamInfos == nil {
		// 解析器还获取到流信息，一般为刚开始解析时未解析到关键帧
		return true
	}

	streamPushers := p.StreamPushers()
	// 查找推流服务中是否包含PS流通道，如果包含则直接将输入的RTP帧发送
	for _, streamPusher := range streamPushers {
		if streamPusher.StreamInfo().MediaType().ID == media.MediaTypePS.ID {

		}
	}

	// 获取PS流子流信息并与推流通道对应
	for _, streamPusher := range streamPushers {
		pusherStreamInfo := streamPusher.StreamInfo()
		mediaType := pusherStreamInfo.MediaType()
		if psStreamInfo := psStreamInfos.GetStreamInfoByMediaType(mediaType.ID); psStreamInfo != nil {
			psStreamId := psStreamInfo.StreamId()
			streamCtx := p.streamContextMap[psStreamId]
			if streamCtx == nil {
				p.streamContextMap[psStreamId] = &streamContext{
					mediaType:     mediaType,
					streamPushers: []*RtpStreamPusher{streamPusher},
				}
			} else {
				streamCtx.streamPushers = append(streamCtx.streamPushers, streamPusher)
			}
		}
	}

	if len(p.streamContextMap) == 0 {
		return true
	}
	defer func() {
		for _, streamCtx := range p.streamContextMap {
			if streamCtx.outputRtpFrame != nil {
				streamCtx.outputRtpFrame.Release()
			}
		}
		for psStreamId := range p.streamContextMap {
			delete(p.streamContextMap, psStreamId)
		}
	}()

	for _, pes := range psFrame.PES() {
		psStreamId := pes.StreamId()
		psStreamInfo := psStreamInfos.GetStreamInfoById(psStreamId)
		if psStreamInfo == nil {
			continue
		}

		streamCtx := p.streamContextMap[psStreamId]
		if streamCtx != nil && !streamCtx.disable {
			switch streamCtx.mediaType.ID {
			case media.MediaTypeH264.ID:
				h264NaluParser := p.getH264NaluParser()
				h264RtpPackager := p.getH264RtpPackager()
				isPrefix := true
				keep = true
				pes.PackageData().Range(func(chunk []byte) bool {
					if !streamCtx.hasData {
						streamCtx.hasData = true
					}
					ok, err := h264NaluParser.Parse(chunk, isPrefix)
					if isPrefix {
						isPrefix = false
					}
					if ok {
						naluHeader, naluBody := h264NaluParser.NALU()
						outputRtpFrame := p.getOutputRtpFrame(streamCtx)
						if outputRtpFrame == nil {
							p.Logger().ErrorWith("H264 RTP打包失败", pool.NewAllocError("RTP帧"))
							streamCtx.disable = true
							return false
						}
						if err := h264RtpPackager.Package(naluHeader, naluBody, streamCtx.outputRtpFrame.Append); err != nil {
							p.Logger().ErrorWith("H264 RTP打包失败", err)
							streamCtx.disable = true
							return false
						}
					}
					if err != nil {
						p.Logger().ErrorWith("解析H264 NALU失败", err)
						if keep = p.keepChooser.OnError(err); !keep {
							p.keepChooser.Reset()
							return false
						}
					}
					return true
				})
				if !keep {
					return false
				}
			default:
				continue
			}
		}
	}

	for _, streamCtx := range p.streamContextMap {
		if streamCtx.hasData && !streamCtx.disable {
			switch streamCtx.mediaType.ID {
			case media.MediaTypeH264.ID:
				h264NaluParser := p.getH264NaluParser()
				h264RtpPackager := p.getH264RtpPackager()
				if h264NaluParser.Complete() {
					naluHeader, naluBody := h264NaluParser.NALU()
					outputRtpFrame := p.getOutputRtpFrame(streamCtx)
					if outputRtpFrame == nil {
						p.Logger().ErrorWith("H264 RTP打包失败", pool.NewAllocError("RTP帧"))
						streamCtx.disable = true
						continue
					}
					if err := h264RtpPackager.Package(naluHeader, naluBody, streamCtx.outputRtpFrame.Append); err != nil {
						p.Logger().ErrorWith("H264 RTP打包失败", err)
						streamCtx.disable = true
						continue
					}
				}
			}
		}
	}

	for _, streamCtx := range p.streamContextMap {
		if outputRtpFrame := streamCtx.outputRtpFrame; outputRtpFrame != nil {
			outputRtpFrame.SetTimestamp(uint32(p.timestamp * 90000 / time.Second))
			for _, streamPusher := range streamCtx.streamPushers {
				if err := streamPusher.send(rtpFramePkg.UseFrame(outputRtpFrame)); err != nil {
					p.Logger().ErrorWith("发送RTP数据失败", err)
					p.RemoveStream(streamPusher.Id())
				}
			}
		}
	}

	return true
}

func (p *HistoryRtpPusher) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if !p.once.CompareAndSwap(false, true) {
		return lifecycle.NewStateClosedError(p.DisplayName())
	}
	p.readSession.Start()
	p.indexGetter.Start()
	p.blockGetter.Start()
	return nil
}

func (p *HistoryRtpPusher) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	defer func() {
		if streamPushers := p.streamPushers.Values(); len(streamPushers) > 0 {
			for _, streamPusher := range streamPushers {
				p.RemoveStream(streamPusher.id)
			}
		}
		p.blockGetter.Shutdown()
		p.indexGetter.Shutdown()
		p.readSession.Shutdown()
		p.psFrameParser.Free()
		p.sendTimer.Stop()
		if p.autoEOFCallback && p.eofCallback != nil {
			p.eofCallback(p, p.channel)
		}
	}()

	var readKeepChooser = rtpServer.NewDefaultKeepChooser(2, 2, nil)

seek:
	for {
		block, err := p.blockGetter.Get()
		if err != nil {
			if err == io.EOF {
				if p.eofCallback != nil {
					p.eofCallback(p, p.channel)
				}
				p.autoEOFCallback = false
				p.Logger().Info("播放完成，停止历史音视频推流")
				if interrupted, seek := p.waitClose(interrupter); interrupted {
					return nil
				} else if seek {
					continue seek
				}
			} else if err == storage.ReadSessionClosedError {
				p.Logger().Warn(err.Error())
				return nil
			}
			// 读取索引或数据错误，连续错误数量达到一定的数量后，关闭推流
			if !readKeepChooser.OnError(err) {
				p.Logger().Warn("读取索引或数据错误次数超过限制，关闭推流")
				return nil
			}
			continue
		}
		readKeepChooser.OnSuccess()

		handlePsFrame := func(psFrame *ps.Frame) (keep, interrupted, seek bool) {
			p.interval = 3600 * time.Second / time.Duration(90000*p.Scale())
			p.timestamp += p.interval
			interrupted, seek = p.waitSend(interrupter)
			if interrupted || seek {
				psFrame.Release()
				return true, interrupted, seek
			}
			return p.handlePsFrame(psFrame), false, false
		}

		for len(block) > 0 {
			ok, err := p.psFrameParser.ParseP(block, &block)
			if err != nil {
				p.Logger().ErrorWith("解析PS帧失败", err)
				if _, is := err.(pool.AllocError); is {
					return nil
				}
				if keep := p.keepChooser.OnError(err); !keep {
					p.keepChooser.Reset()
					return nil
				}
			}
			if ok {
				if keep, interrupted, seek := handlePsFrame(p.psFrameParser.Take()); !keep || interrupted {
					return nil
				} else if seek {
					continue seek
				}
			}
		}
		if p.psFrameParser.Complete() {
			if keep, interrupted, seek := handlePsFrame(p.psFrameParser.Take()); !keep || interrupted {
				return nil
			} else if seek {
				continue seek
			}
		}
		p.psFrameParser.Reset()
	}
}

func (p *HistoryRtpPusher) StartPush() {
	if !utils.ChanTryPush(p.pushSignal, struct{}{}) {
		p.Logger().Warn("开始推流信号繁忙")
	}
}

func (p *HistoryRtpPusher) Pause() {
	if !utils.ChanTryPush(p.pauseSignal, struct{}{}) {
		p.Logger().Warn("暂停信号繁忙")
	}
}

func (p *HistoryRtpPusher) Seek(t int64) error {
	if t < 0 {
		t = 0
	}
	if p.startTime+t > p.endTime {
		t = p.endTime - p.startTime
	}
	if !utils.ChanTryPush(p.seekSignal, p.startTime+t) {
		p.Logger().Warn("定位信号繁忙")
	}
	return nil
}

func (p *HistoryRtpPusher) Scale() float64 {
	scaleU := p.scale.Load()
	return *(*float64)(unsafe.Pointer(&scaleU))
}

func (p *HistoryRtpPusher) SetScale(scale float64) error {
	if scale > 8 || scale < 0.1 {
		return p.Logger().ErrorMsg("播放倍速超过了限制")
	}
	p.scale.Store(*(*uint64)(unsafe.Pointer(&scale)))
	return nil
}

func (p *HistoryRtpPusher) Info() map[string]any {
	infos := make([]map[string]any, 0)
	p.streamPushers.Range(func(id uint64, streamPusher *RtpStreamPusher) bool {
		infos = append(infos, streamPusher.Info())
		return true
	})
	return map[string]any{
		"id":            p.id,
		"startTime":     p.startTime,
		"endTime":       p.endTime,
		"streamPushers": infos,
	}
}
