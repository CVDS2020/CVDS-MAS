package meta

import (
	"errors"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lifecycle/retry"
	taskPkg "gitee.com/sy_183/common/lifecycle/task"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/slice"
	"gitee.com/sy_183/cvds-mas/storage"
	"gitee.com/sy_183/cvds-mas/storage/file/allocator"
	"io"
	"os"
	"sort"
	"sync"
	"time"
	"unsafe"
)

// Seek 结构体携带了 os.File.Seek 需要的参数
type Seek struct {
	Offset int64
	Whence int
}

type ReadRequest struct {
	Buf    []byte
	Seek   *Seek
	Full   bool
	Future storage.Future[*ReadResponse]
}

type ReadResponse struct {
	Request *ReadRequest
	Data    []byte

	// 修改文件指针位置花费的时间
	SeekElapse time.Duration
	ReadElapse time.Duration

	Err error
}

type WriteRequest struct {
	Data   []byte
	Seek   *Seek
	Future storage.Future[*WriteResponse]
}

type WriteResponse struct {
	Request *WriteRequest
	// 文件写入的大小
	Size int64

	// 修改文件指针位置花费的时间
	SeekElapse  time.Duration
	WriteElapse time.Duration

	// 写入错误
	Err error
}

type IndexBlockEntry struct {
	IndexBlockHeader
	Loc        int
	IndexBlock *IndexBlock
}

func (e *IndexBlockEntry) Offset(blockSize uint32) int64 {
	return int64(e.Loc+1) * int64(blockSize)
}

type sortableIndexBlockHeaders struct {
	*container.Queue[IndexBlockEntry]
}

func (hs sortableIndexBlockHeaders) Less(i, j int) bool {
	return hs.Pointer(i).Seq < hs.Pointer(j).Seq
}

type IndexBlockManager struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	maxBlocks int
	maxCached int
	blockSize uint32

	file string
	fp   *os.File

	blockAllocator *allocator.BitMap
	blockEntries   *container.Queue[IndexBlockEntry]
	blockDeleted   *allocator.BitMap

	latestBlocks        *container.Queue[IndexBlock]
	latestBlocksRaw     []byte
	earliestBlock       *IndexBlock
	earliestBlockRaw    []byte
	writableLatestBlock *IndexBlock

	writeMerge     int
	writeMergePool pool.DataPool

	mu *sync.Mutex

	refreshed    int
	notRefreshed int
	refreshing   int

	readerWriter *taskPkg.TaskExecutor
	flusher      *taskPkg.TaskExecutor

	log.AtomicLogger
}

func (m *IndexBlockManager) loadLatestBlocks() error {
	i, total, cached := 0, m.blockEntries.Len(), m.latestBlocks.Cap()
	if total == 0 || cached == 0 {
		return nil
	}
	if cached < total {
		i = total - cached
	} else if cached > total {
		cached = total
	}
	cache, _ := m.latestBlocks.Slice(-1, cached)
	raw := cache[0].raw[:cached*int(m.blockSize)]
	loc, start := -1, i

	doLoad := func() error {
		startEntry := m.blockEntries.Pointer(start)
		res := m.readSync(&ReadRequest{
			Buf:  raw[:(i-start)*int(m.blockSize)],
			Seek: &Seek{Offset: startEntry.Offset(m.blockSize)},
			Full: true,
		})
		if res.Err != nil {
			return res.Err
		}
		for j := start; j < i; j++ {
			cur := m.latestBlocks.Use()
			if err := cur.ResetWith(nil, true); err != nil {
				return err
			}
			if *cur.IndexBlockHeader != m.blockEntries.Pointer(j).IndexBlockHeader {
				return errors.New("")
			}
		}
		raw = raw[(i-start)*int(m.blockSize):]
		start = i
		return nil
	}

	for ; i < total; i++ {
		entry := m.blockEntries.Pointer(i)
		if loc != -1 {
			if entry.Loc != loc+1 {
				if err := doLoad(); err != nil {
					return err
				}
			}
		}
		loc = entry.Loc
	}
	if i > start {
		if err := doLoad(); err != nil {
			return err
		}
	}
	m.preAddIndex()
	return nil
}

func (m *IndexBlockManager) loadEarliestBlock() error {
	earliestEntry := m.blockEntries.HeadPointer()
	if earliestEntry == nil {
		return nil
	}
	latestHead := m.latestBlocks.HeadPointer()
	if latestHead != nil && latestHead.Seq == earliestEntry.Seq {
		m.earliestBlock = latestHead
		return nil
	}
	res := m.readSync(&ReadRequest{
		Buf:  m.earliestBlockRaw,
		Seek: &Seek{Offset: earliestEntry.Offset(m.blockSize)},
		Full: true,
	})
	if res.Err != nil {
		return res.Err
	}
	if err := m.earliestBlock.ResetWith(m.earliestBlockRaw, true); err != nil {
		return err
	}
	if *m.earliestBlock.IndexBlockHeader != earliestEntry.IndexBlockHeader {
		return errors.New("")
	}
	return nil
}

func (m *IndexBlockManager) initBlockCache() {
	m.latestBlocksRaw = make([]byte, m.maxCached*int(m.blockSize))
	prefix, _ := m.latestBlocks.Slice(-1, -1)
	for i, block := range prefix {
		block.Reset(m.latestBlocksRaw[i*int(m.blockSize) : (i+1)*int(m.blockSize)])
	}
	m.earliestBlockRaw = make([]byte, m.blockSize)
}

//func (m *IndexBlockManager) loadEarliestAndLatestBlock(fp *os.File) (err error) {
//	var recoveries []func()
//	{
//		blockCache, blockHeaderMap := m.blockCache, m.blockHeaderMap
//		recoveries = append(recoveries, func() {
//			m.blockCache = blockCache
//			m.blockHeaderMap = blockHeaderMap
//		})
//	}
//	defer func() {
//		if err != nil {
//			for _, r := range recoveries {
//				r()
//			}
//		}
//	}()
//
//	earliestEntry, latestEntry := m.blockEntries.HeadPointer(), m.blockEntries.TailPointer()
//	if earliestEntry != nil {
//		var entries []*IndexBlockEntry
//		if earliestEntry != earliestEntry {
//			entries = []*IndexBlockEntry{earliestEntry, latestEntry}
//		} else {
//			entries = []*IndexBlockEntry{earliestEntry}
//		}
//		for i, entry := range entries {
//			seek := entry.Offset(m.blockSize)
//			if _, err := fp.Seek(seek, 0); err != nil {
//				return m.Logger().ErrorWith("seek index block file error", err, log.String("file", m.file), log.Int64("seek", seek))
//			}
//			m.blockCache = m.blockCache[:len(m.blockCache)+1]
//			m.blockHeaderMap = append(m.blockHeaderMap, i*(m.blockEntries.Len()-1))
//			block := &m.blockCache[len(m.blockCache)-1]
//			if _, err := block.ReadFrom(fp); err != nil {
//				if ibe, is := err.(IndexBlockError); is {
//					ibe.SetLoc(entry.Loc)
//					return m.Logger().ErrorWith("parse index block file error", err, log.String("file", m.file), log.Int64("offset", seek))
//				}
//				return m.Logger().ErrorWith("read index block file error", err, log.String("file", m.file), log.Int64("offset", seek))
//			}
//			entry.IndexBlock = block
//			recoveries = append(recoveries, func() {
//				entry.IndexBlock = nil
//			})
//		}
//	} else {
//		m.blockEntries.Push(IndexBlockEntry{IndexBlockHeader: IndexBlockHeader{Seq: 1}})
//		m.blockCache = m.blockCache[:len(m.blockCache)+1]
//		m.blockHeaderMap = append(m.blockHeaderMap, 0)
//		block := &m.blockCache[len(m.blockCache)-1]
//		*block.IndexBlockHeader = latestEntry.IndexBlockHeader
//		block.ResetWith(nil)
//	}
//	return nil
//}

func (m *IndexBlockManager) scanFile() error {
	// scan file header, check magic code and block size
	//if _, err := m.fp.Seek(0, 0); err != nil {
	//	return m.Logger().ErrorWith("seek to index block file start error", err, log.String("file", m.file))
	//}
	buf := m.latestBlocksRaw[:pageSize]
	var fileHeader IndexBlockFileHeader
	res := m.readSync(&ReadRequest{
		Buf:  buf,
		Seek: &Seek{Offset: 0},
		Full: true,
	})
	if res.Err != nil {
		return m.Logger().ErrorWith("read index block file header error", res.Err, log.String("file", m.file))
	}
	copy(slice.Make[byte](unsafe.Pointer(&fileHeader), indexBlockFileHeaderSize, indexBlockFileHeaderSize), buf)
	if fileHeader.Magic != IndexBlockFileMagic {
		err := &IndexBlockFileMagicError{Magic: fileHeader.Magic}
		m.Logger().Error(err.Error())
		return err
	}
	if fileHeader.BlockSize < MinIndexBlockSize || fileHeader.BlockSize > MaxIndexBlockSize {
		err := &IndexBlockBlockSizeError{BlockSize: fileHeader.BlockSize}
		m.Logger().Error(err.Error())
		return err
	}
	m.blockSize = fileHeader.BlockSize
	if pageSize != int(fileHeader.BlockSize) {
		if _, err := m.fp.Seek(0, int(fileHeader.BlockSize)); err != nil {
			return m.Logger().ErrorWith("seek index block file error", err, log.String("file", m.file), log.Uint32("seek", fileHeader.BlockSize))
		}
	}

	// scan and check index block
	checker := NewIndexBlock(nil)
	offset, loc := int64(indexBlockFileHeaderSize), 0
scanEnd:
	for {
		res := m.readSync(&ReadRequest{
			Buf: m.latestBlocksRaw,
		})
		if res.Err != nil {
			if res.Err == io.EOF {
				break scanEnd
			}
			return m.Logger().ErrorWith("read index block file error", res.Err, log.String("file", m.file), log.Int64("offset", offset))
		}
		for i := 0; i < len(res.Data); i += int(fileHeader.BlockSize) {
			raw := res.Data[i:]
			if len(raw) < int(fileHeader.BlockSize) {
				break scanEnd
			}
			err := checker.ResetWith(raw[:fileHeader.BlockSize], true)
			if err != nil {
				if ibe, is := err.(IndexBlockError); is {
					ibe.SetLoc(loc)
				}
				return m.Logger().ErrorWith("parse index block file error", err, log.String("file", m.file), log.Int64("offset", offset))
			}
			if loc >= m.maxBlocks {
				break scanEnd
			}
			if checker.Seq != 0 {
				m.blockAllocator.AllocIndex(int64(loc))
				m.blockEntries.Push(IndexBlockEntry{
					IndexBlockHeader: *checker.IndexBlockHeader,
					Loc:              loc,
				})
			}
			loc++
		}
		offset += int64(len(res.Data))
	}
	remainPrefix, remainSuffix := m.blockEntries.Slice(m.blockEntries.Len(), -1)
	for _, entries := range [][]IndexBlockEntry{remainPrefix, remainSuffix} {
		for i := range entries {
			if loc < m.blockEntries.Len() {
				entries[i].Loc = loc
				loc++
			} else {
				entries[i].Loc = int(m.blockAllocator.Alloc())
			}
		}
	}

	sort.Sort(sortableIndexBlockHeaders{Queue: m.blockEntries})
	var last *IndexBlockEntry
	prefix, suffix := m.blockEntries.Slice(-1, -1)
	for _, checking := range [][]IndexBlockEntry{prefix, suffix} {
		for i := 0; i < len(checking); i++ {
			cur := &checking[i]
			if last != nil {
				if cur.Seq == last.Seq {
					err := &IndexBlockSeqRepeatError{
						LastIndexBlockInfo: IndexBlockInfo{Seq: int64(last.Seq), Loc: last.Loc},
						IndexBlockInfo:     IndexBlockInfo{Seq: int64(cur.Seq), Loc: cur.Loc},
					}
					m.Logger().Error(err.Error(), log.String("file", m.file))
					return err
				}
				if cur.Start < last.End {
					err := &AdjacentIndexBlockTimestampError{
						LastEnd:            last.End,
						Start:              cur.Start,
						LastIndexBlockInfo: IndexBlockInfo{Seq: int64(last.Seq), Loc: last.Loc},
						IndexBlockInfo:     IndexBlockInfo{Seq: int64(cur.Seq), Loc: cur.Loc},
					}
					m.Logger().Error(err.Error(), log.String("file", m.file))
					return err
				}
			}
			last = cur
		}
	}

	m.initBlockCache()

	if err := m.loadLatestBlocks(); err != nil {
		return err
	}

	if err := m.loadEarliestBlock(); err != nil {
		return err
	}

	return nil
}

func (m *IndexBlockManager) createFile(fp *os.File) error {
	if _, err := fp.Seek(0, 0); err != nil {
		return m.Logger().ErrorWith("seek to index block file start error", err, log.String("file", m.file))
	}
	fileHeader := IndexBlockFileHeader{
		Magic:     IndexBlockFileMagic,
		BlockSize: m.blockSize,
	}
	buf := make([]byte, m.blockSize)
	copy(buf, slice.Make[byte](unsafe.Pointer(&fileHeader), indexBlockFileHeaderSize, indexBlockFileHeaderSize))
	if _, err := fp.Write(slice.Make[byte](unsafe.Pointer(&fileHeader), indexBlockFileHeaderSize, indexBlockFileHeaderSize)); err != nil {
		return m.Logger().ErrorWith("write index block file header error", err, log.String("file", m.file))
	}
	m.initBlockCache()
	m.preAddIndex()
	m.earliestBlock = m.latestBlocks.TailPointer()
	return nil
}

func (m *IndexBlockManager) loadFile() (err error) {
	fp, err := os.OpenFile(m.file, os.O_CREATE|os.O_RDWR|os.O_SYNC, 0644)
	if err != nil {
		return m.Logger().ErrorWith("open index block file error", err, log.String("file", m.file))
	}
	m.fp = fp
	defer func() {
		if err != nil {
			if err := fp.Close(); err != nil {
				m.Logger().ErrorWith("close index block file error", err, log.String("file", m.file))
			}
			m.fp = nil
			return
		}
	}()

	fileInfo, err := fp.Stat()
	if err != nil {
		return m.Logger().ErrorWith("get file info error", err, log.String("file", m.file))
	}
	if fileInfo.Size() == 0 {
		if err := m.createFile(fp); err != nil {
			return err
		}
		return nil
	}

	if err := m.scanFile(); err != nil {
		return err
	}

	return
}

func (m *IndexBlockManager) doRead(req *ReadRequest) *ReadResponse {
	res := &ReadResponse{Request: req}
	start := time.Now()
	if req.Seek != nil {
		_, err := m.fp.Seek(req.Seek.Offset, req.Seek.Whence)
		res.SeekElapse = time.Since(start)
		start = start.Add(res.SeekElapse)
		if err != nil {
			res.Err = err
			return res
		}
	}
	reader := func(r io.Reader, buf []byte) (n int, err error) { return r.Read(buf) }
	if req.Full {
		reader = io.ReadFull
	}
	n, err := reader(m.fp, req.Buf)
	res.ReadElapse = time.Since(start)
	if err != nil {
		res.Err = err
	}
	res.Data = req.Buf[:n]
	return res
}

func (m *IndexBlockManager) doWrite(req *WriteRequest) *WriteResponse {
	res := &WriteResponse{Request: req}
	start := time.Now()
	if req.Seek != nil {
		_, err := m.fp.Seek(req.Seek.Offset, req.Seek.Whence)
		res.SeekElapse = time.Since(start)
		start = start.Add(res.SeekElapse)
		if err != nil {
			res.Err = err
			return res
		}
	}
	n, err := m.fp.Write(req.Data)
	res.WriteElapse = time.Since(start)
	res.Size = int64(n)
	if err != nil {
		res.Err = err
	}
	return res
}

//func (m *IndexBlockManager) readerWriterRun(interrupter chan struct{}) error {
//	defer func() {
//		for {
//			select {
//			case req := <-m.readRequestChan:
//				req.Future.Response(m.doRead(req))
//			case req := <-m.writeRequestChan:
//				req.Future.Response(m.doWrite(req))
//			default:
//				return
//			}
//		}
//	}()
//	for {
//		select {
//		case req := <-m.readRequestChan:
//			req.Future.Response(m.doRead(req))
//		case req := <-m.writeRequestChan:
//			req.Future.Response(m.doWrite(req))
//		case <-interrupter:
//			return nil
//		}
//	}
//}

func (m *IndexBlockManager) readSync(req *ReadRequest) *ReadResponse {
	var res *ReadResponse
	m.readerWriter.Sync(taskPkg.Func(func() {
		res = m.doRead(req)
	}))
	return res
}

func (m *IndexBlockManager) writeSync(req *WriteRequest) *WriteResponse {
	var res *WriteResponse
	m.readerWriter.Sync(taskPkg.Func(func() {
		res = m.doWrite(req)
	}))
	return res
}

func (m *IndexBlockManager) run(interrupter chan struct{}) error {
	if err := retry.MakeRetry(retry.Retry{
		Do:          m.loadFile,
		MaxRetry:    -1,
		Interrupter: interrupter,
	}).Todo(); err == retry.InterruptedError {
		m.Logger().Warn("load index block file interrupt")
		return nil
	}

	return nil
}

func (m *IndexBlockManager) preAddIndex() (addable bool) {
	latest := m.latestBlocks.TailPointer()
	if m.refreshing != 0 && m.refreshed+m.refreshing == m.blockEntries.Len() && m.writableLatestBlock.Seq != 0 {
		//     .---------- latestBlocks -----------.
		//     |     .-------- refreshing ---------|
		// ____|_____|_____________________________|
		// |         |                             |
		// 0     refreshed                      latest
		//                                writableLatestBlock
		//
		//     .---------------- latestBlocks ----------------.
		//     |----------------- refreshing -----------------|
		//     |______________________________________________|
		//     |         |                                    |
		// refreshed     0                                 latest
		//
		latest = m.writableLatestBlock
	}
	if latest == nil || latest.Full() {
		if m.blockEntries.Len() == m.blockEntries.Cap() {
			return false
		}
		if m.latestBlocks.Len() == m.latestBlocks.Cap() {
			notRefreshed := m.blockEntries.Len() - m.refreshed
			if notRefreshed == m.latestBlocks.Len() {
				//           .-------- latestBlocks(full) ---------.
				//           |----------- notRefreshed ------------|
				//           |-------- refreshing ---------.       |
				// __________|_____________________________|_______|
				// |         |                                     |
				// 0     refreshed                              latest
				//
				//     .----------------- latestBlocks(full) -----------------.
				//     |-------------------- notRefreshed --------------------|
				//     |----------------- refreshing -----------------.       |
				//     |______________________________________________|_______|
				//     |         |                                            |
				// refreshed     0                                         latest
				//
				return false
			}
			headBlock := m.latestBlocks.HeadPointer()
			if headBlock == m.earliestBlock {
				copy(m.earliestBlockRaw, headBlock.raw)
				m.earliestBlock.ResetWith(m.earliestBlockRaw, false)
			}
			m.latestBlocks.Pop()
		}
		seq := uint64(1)
		if latest != nil {
			seq += latest.Seq
		}
		latestEntry := m.blockEntries.Use()
		latestEntry.IndexBlockHeader = IndexBlockHeader{Seq: seq}
		latest = m.latestBlocks.Use()
		*latest.IndexBlockHeader = latestEntry.IndexBlockHeader
		latest.ResetWith(nil, false)
		return true
	}
	return true
}

func (m *IndexBlockManager) FirstIndex() {

}

func (m *IndexBlockManager) AddIndex(index *Index) {
	m.mu.Lock()
	if !m.preAddIndex() {
		m.mu.Unlock()
		return
	}
	latestBlock := m.latestBlocks.TailPointer()
	if m.refreshing != 0 && m.refreshing+m.refreshed == m.blockEntries.Len() {
		if m.writableLatestBlock.Seq == 0 {
			*m.writableLatestBlock.IndexBlockHeader = *latestBlock.IndexBlockHeader
			if latestBlock.Len != 0 {
				m.writableLatestBlock.Len = latestBlock.Len - 1
				m.writableLatestBlock.ResetWith(nil, false)
				latest := latestBlock.Get(int(latestBlock.Len) - 1)
				m.writableLatestBlock.Append(&latest, false)
			} else {
				m.writableLatestBlock.ResetWith(nil, false)
			}
			latestBlock = m.writableLatestBlock
		}
	}
	if latestBlock.Len == 0 {
		if m.latestBlocks.Len() > 1 {
			latestBack := m.latestBlocks.Pointer(m.latestBlocks.Len() - 2)
			if index.Seq < latestBack.Seq {
				//panic(&IndexBlockAdjacentIndexSeqError{
				//	IndexBlockInfo: IndexBlockInfo{Seq: int64(latestBack.Seq)},
				//})
			}
			if index.Start < latestBack.End {

			}
		}
	}
	latestBlock.Append(index, true)

}

func (m *IndexBlockManager) DeleteIndex() {

}

func (m *IndexBlockManager) FlushLatest(future *storage.Future[error]) error {
	if ok, err := m.flusher.Try(taskPkg.Func(func() {
		m.mu.Lock()
		if m.refreshed == m.blockEntries.Len() {
			m.mu.Unlock()
			future.Response(nil)
			return
		}
		m.refreshing = m.blockEntries.Len() - m.refreshed
		prefixEntries, suffixEntries := m.blockEntries.Slice(m.refreshed, -1)
		prefixBlocks, suffixBlocks := m.latestBlocks.Slice(m.refreshed, -1)
		m.mu.Unlock()

		var err error
		defer func() {
			m.mu.Lock()

			m.mu.Unlock()
			future.Response(err)
		}()

		entryChunks := slice.Chunks[IndexBlockEntry]{prefixEntries, suffixEntries}
		blockChunks := slice.Chunks[IndexBlock]{prefixBlocks, suffixBlocks}

		var start, end int
		var loc = -1
		var doFlush = func() error {
			{
				var start, end = start, end
				var p uintptr
				var buf, tmp *pool.Data
				var lastOff int64
				var doFlush = func(flushAll bool) error {
					startEntry := entryChunks.Pointer(start)
					startBlock := blockChunks.Pointer(start)
					raw := startBlock.Raw()[:(end-start)*int(m.blockSize)]
					var last *pool.Data
					var this []byte
					if len(raw) >= m.writeMerge {
						if buf != nil {
							last, this = buf, raw
						} else if tmp != nil {
							last, this = tmp, raw
						}
					} else if buf == nil {
						if tmp != nil && len(tmp.Data)+len(raw) > m.writeMerge {
							last, this = tmp, raw
						} else {
							buf = m.writeMergePool.AllocCap(0, uint(m.writeMerge))
							if tmp != nil {
								buf.Data = append(append(buf.Data, tmp.Data...), raw...)
								if len(buf.Data) == m.writeMerge {
									last = buf
									buf = nil
								}
							} else {
								tmp = pool.NewData(raw)
								lastOff = startEntry.Offset(m.blockSize)
							}
						}
					} else {
						if len(buf.Data)+len(raw) > m.writeMerge {
							last, this = buf, raw
						} else {
							buf.Data = append(buf.Data, raw...)
							if len(buf.Data) == m.writeMerge {
								last = buf
								buf = nil
							}
						}
					}
					if flushAll && last == nil {
						if buf != nil {
							last = buf
						} else if tmp != nil {
							last = tmp
						}
					}
					if last != nil {
						res := m.writeSync(&WriteRequest{
							Data: last.Data,
							Seek: &Seek{Offset: lastOff},
						})
						last.Release()
						if res.Err != nil {
							return res.Err
						}
						if this != nil {
							res := m.writeSync(&WriteRequest{
								Data: this,
								Seek: &Seek{Offset: startEntry.Offset(m.blockSize)},
							})
							if res.Err != nil {
								return res.Err
							}
						}
					}
					start = end
					return nil
				}
				for _, blocks := range blockChunks.Cut(start, end) {
					for _, block := range blocks {
						cp := uintptr(unsafe.Pointer(&block.Raw()[0]))
						if p != 0 {
							if cp != p+uintptr(m.blockSize) {
								if err := doFlush(false); err != nil {
									return err
								}
							}
						}
						p = cp
						end++
					}
				}
				if end > start {
					if err := doFlush(true); err != nil {
						return err
					}
				}
			}
			start = end
			return nil
		}
		for _, entries := range entryChunks {
			for _, entry := range entries {
				if loc != -1 {
					if entry.Loc != loc+1 {
						if err = doFlush(); err != nil {
							return
						}
					}
				}
				loc = entry.Loc
				end++
			}
		}
		if end > start {
			if err = doFlush(); err != nil {
				return
			}
		}
	})); !ok || err != nil {
		return FlushLatestIndexTaskFullError
	}
	return nil
}

func (m *IndexBlockManager) FlushEarliest() error {
	return nil
}
