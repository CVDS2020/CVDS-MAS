package channel

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
	"testing"
	"time"
)

func TestH264PesRtpPackager(t *testing.T) {
	packager := NewH264PesRtpPackager()
	parser := ps.NewFrameParser()

	fp, err := os.Open("C:\\Users\\suy\\Documents\\Language\\Go\\cvds-cmu\\data\\44010200491320000123_44010200491320000123\\C2074-车厢1-转向架-20230314200939-2.mpg")
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IP{192, 168, 80, 1}, Port: 5004})
	if err != nil {
		t.Fatal(err)
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

		streamInfos := parser.StreamInfos()
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
		for _, pes := range frame.PES() {
			if pes.StreamId() == streamInfo.StreamId() {
				packager.Package(pes, rtpFrame.Append)
			}
		}
		packager.Complete(rtpFrame.Append)
		rtpFrame.SetPayloadType(media.MediaTypeH264.PT)
		rtpFrame.SetTimestamp(timestamp)
		timestamp += 3600
		if rtpFrame.Len() > 0 {
			rtpFrame.Packet(rtpFrame.Len() - 1).SetMarker(true)
		}

		frame.AddRelation(rtpFrame)
		rtpFrame.Range(func(i int, packet rtp.Packet) bool {
			packet.SetSequenceNumber(seq)
			seq++
			packet.WriteTo(rtpWriteBuffer)
			if _, err := conn.Write(rtpWriteBuffer.Bytes()); err != nil {
				t.Fatal(err)
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
					if parser.Complete() {
						handleFrame(parser.Take())
					}
					parser.Free()
					return true
				}
				t.Fatal(err)
			}

			remain := data.Data[:n]
			for len(remain) > 0 {
				ok, err := parser.ParseP(remain, &remain)
				if ok {
					handleFrame(parser.Take())
				}
				if err != nil {
					data.Release()
					t.Fatal(err)
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
