package storage

type IndexChecker struct {
	startSeq       uint64
	endSeq         uint64
	startTime      int64
	endTime        int64
	seqErrorCount  int
	timeErrorCount int
}

func NewIndexChecker(startTime, endTime int64, startSeq, endSeq uint64) *IndexChecker {
	return &IndexChecker{
		startSeq:  startSeq,
		endSeq:    endSeq,
		startTime: startTime,
		endTime:   endTime,
	}
}

func (c *IndexChecker) Check(index Index) bool {
	if index.Start() > index.End() {
		c.timeErrorCount++
		return false
	}
	if c.startSeq != 0 {
		if index.Seq() <= c.startSeq {
			c.seqErrorCount++
			return false
		}
	}
	c.startSeq = index.Seq()
	if c.endSeq != 0 {
		if index.Seq() >= c.endSeq {
			c.seqErrorCount++
			return false
		}
	}
	if c.startTime != 0 {
		if index.Start() < c.startTime {
			c.timeErrorCount++
			return false
		}
	}
	c.startTime = index.End()
	if c.endTime != 0 {
		if index.End() > c.endTime {
			c.timeErrorCount++
			return false
		}
	}
	return true
}

func (c *IndexChecker) SeqErrorCount() int {
	return c.seqErrorCount
}

func (c *IndexChecker) TimeErrorCount() int {
	return c.timeErrorCount
}
