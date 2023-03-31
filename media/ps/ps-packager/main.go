package main

import (
	"bytes"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/media/ps"
	rtpFramePkg "gitee.com/sy_183/rtp/frame"
	"gitee.com/sy_183/rtp/rtp"
	"io"
	"math"
	"net"
	"os"
	"time"
)

func main() {
	psFrameParser := ps.NewFrameParser()
	psRtpPackger := ps.NewRtpPackager(0)

	fp, err := os.Open("C:\\Users\\suy\\Documents\\Language\\Go\\cvds-cmu\\data\\44010200491320000123_44010200491320000123\\C2074-车厢1-转向架-20230314200939-2.mpg")
	if err != nil {
		panic(err)
	}
	defer fp.Close()

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IP{192, 168, 80, 1}, Port: 5004})
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	dataPool := pool.NewStaticDataPool(1470, pool.ProvideSlicePool[*pool.Data], pool.WithLimit(math.MaxInt64))
	rtpFramePool := pool.ProvideSlicePool(rtpFramePkg.NewDefaultFrame, pool.WithLimit(math.MaxInt64))
	rtpWriteBuffer := &bytes.Buffer{}

	var chunks []*pool.Data
	var timestamp uint32
	var seq uint16

	handleFrame := func(frame *ps.Frame) {
		for _, chunk := range chunks {
			frame.AddRelation(chunk)
		}
		chunks = chunks[:0]

		streamInfos := psFrameParser.StreamInfos()
		if streamInfos == nil {
			frame.Release()
			return
		}

		streamInfo := streamInfos.GetStreamInfoByMediaType(media.MediaTypeH264.ID)
		if streamInfo == nil {
			frame.Release()
			return
		}

		rtpFrame := rtpFramePool.Get().Use()
		if err := psRtpPackger.Package(frame, rtpFrame.Append); err != nil {
			panic(err)
		}

		rtpFrame.SetPayloadType(media.MediaTypePS.PT)
		rtpFrame.SetTimestamp(timestamp)
		timestamp += 3600

		frame.AddRelation(rtpFrame)
		rtpFrame.Range(func(i int, packet rtp.Packet) bool {
			packet.SetSequenceNumber(seq)
			seq++
			packet.WriteTo(rtpWriteBuffer)
			if _, err := conn.Write(rtpWriteBuffer.Bytes()); err != nil {
				panic(err)
			}
			rtpWriteBuffer.Reset()
			//fmt.Println(packet)
			return true
		})
		frame.Release()
		time.Sleep(time.Millisecond * 40)
	}

	for {
		if func() bool {
			data := dataPool.Alloc(1470)
			defer data.Release()

			n, err := fp.Read(data.Data)
			if err != nil {
				if err == io.EOF {
					if psFrameParser.Complete() {
						handleFrame(psFrameParser.Take())
					}
					psFrameParser.Free()
					return true
				}
				panic(err)
			}

			remain := data.Data[:n]
			for len(remain) > 0 {
				ok, err := psFrameParser.ParseP(remain, &remain)
				if ok {
					handleFrame(psFrameParser.Take())
				}
				if err != nil {
					data.Release()
					panic(err)
				}
				if !ok {
					chunks = append(chunks, data.Use())
				}
			}

			return false
		}() {
			return
		}
	}
}
