package ps

import (
	"bufio"
	"bytes"
	"fmt"
	ioUtils "gitee.com/sy_183/common/utils/io"
	"gitee.com/sy_183/cvds-mas/media/h264"
	"io"
	"os"
	"strings"
	"testing"
)

func TestParser(t *testing.T) {
	file, err := os.Open("C:\\Users\\suy\\Documents\\Language\\Go\\cvds-cmu\\data\\test\\test-20230329144733-1.mpg")
	if err != nil {
		t.Fatal(err)
	}
	logFile, err := os.OpenFile("ps.info", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	logWriter := bufio.NewWriter(logFile)
	defer logWriter.Flush()

	parser := Parser{}
	parser.SetPSH(new(PSH))
	parser.SetSYS(new(SYS))
	parser.SetPSM(new(PSM))
	parser.SetPES(new(PES))
	for {
		buf := make([]byte, 1470)
		n, err := file.Read(buf)
		if err != nil {
			if err == io.EOF {
				return
			}
			t.Fatal(err)
		}
		data := buf[:n]

		p := data[:n]
		lastPTS := make(map[uint8]uint64)
		for len(p) > 0 {
			ok, remain, err := parser.Parse(p)
			p = remain
			if err != nil {
				t.Fatal(err)
			} else if ok {
				switch pack := parser.Pack().(type) {
				case *PSH:
					fmt.Fprintf(logWriter, "[PSH] Clock: %d\n", pack.SystemClockReferenceBase())
					pack.Clear()
					parser.SetPSH(new(PSH))
				case *SYS:
					for i, info := range pack.streamInfos {
						if i == 0 {
							fmt.Fprintf(logWriter, "[SYS] StreamId: %02x\n", info.StreamId())
						} else {
							fmt.Fprintf(logWriter, "      StreamId: %02x\n", info.StreamId())
						}
					}
					pack.Clear()
					parser.SetSYS(new(SYS))
				case *PSM:
					for i, info := range pack.elementaryStreamInfos {
						var typeName string
						switch info.typ {
						case StreamTypeVideoMPEG1:
							typeName = "VideoMPEG1"
						case StreamTypeVideoMPEG2:
							typeName = "VideoMPEG2"
						case StreamTypeAudioMPEG1:
							typeName = "AudioMPEG1"
						case StreamTypeAudioMPEG2:
							typeName = "AudioMPEG2"
						case StreamTypeAudioAAC:
							typeName = "AudioAAC"
						case StreamTypeVideoMPEG4:
							typeName = "VideoMPEG4"
						case StreamTypeVideoH264:
							typeName = "VideoH264"
						case StreamTypeVideoH265:
							typeName = "VideoH265"
						case StreamTypeVideoSVAC:
							typeName = "VideoSVAC"
						case StreamTypeAudioAC3:
							typeName = "AudioAC3"
						case StreamTypeAudioDTS:
							typeName = "AudioDTS"
						case StreamTypeAudioLPCM:
							typeName = "AudioLPCM"
						case StreamTypeAudioG711A:
							typeName = "AudioG711A"
						case StreamTypeAudioG711U:
							typeName = "AudioG711U"
						case StreamTypeAudioG7221:
							typeName = "AudioG7221"
						case StreamTypeAudioG7231:
							typeName = "AudioG7231"
						case StreamTypeAudioG729:
							typeName = "AudioG729"
						case StreamTypeAudioSVAC:
							typeName = "AudioSVAC"
						}
						if i == 0 {
							fmt.Fprintf(logWriter, "[PSM] Stream#%d: {type: %s, id: %02x}\n", i, typeName, info.id)
						} else {
							fmt.Fprintf(logWriter, "      Stream#%d: {type: %s, id: %02x}\n", i, typeName, info.id)
						}
					}
					pack.Clear()
					parser.SetPSM(new(PSM))
				case *PES:
					var naluType uint8
					if pack.PackageData().Size() >= 5 {
						prefixWriter := ioUtils.Writer{Buf: make([]byte, 5)}
						pack.PackageData().Range(func(chunk []byte) bool {
							if prefixWriter.WriteBytes(chunk) == 0 {
								return false
							}
							return true
						})
						if prefix := prefixWriter.Bytes(); bytes.HasPrefix(prefix, []byte{0, 0, 1}) {
							naluType = h264.NALUHeader(prefix[3]).Type()
						} else if bytes.HasPrefix(prefix, []byte{0, 0, 0, 1}) {
							naluType = h264.NALUHeader(prefix[4]).Type()
						}
					}
					fields := []string{
						fmt.Sprintf("StreamId: %02x", pack.StreamId()),
						fmt.Sprintf("DataLength: %d", pack.PackageData().Size()),
					}
					if pack.PTS_DTS_Flags() == 0b10 {
						fields = append(fields, fmt.Sprintf("PTS: %d", pack.PTS()))
						last := lastPTS[pack.StreamId()]
						if last != 0 {
							fields = append(fields, fmt.Sprintf("PTS-ADDED: %d", pack.PTS()-last))
						}
						lastPTS[pack.StreamId()] = pack.PTS()
					} else if pack.PTS_DTS_Flags() == 0b11 {
						fields = append(fields, fmt.Sprintf("PTS: %d", pack.PTS()), fmt.Sprintf("DTS: %d", pack.DTS()))
					}
					if naluType != 0 {
						fields = append(fields, fmt.Sprintf("NALU-Type: %d", naluType))
					}
					fmt.Fprintf(logWriter, "  [PES] %s\n", strings.Join(fields, ", "))
					pack.Clear()
					parser.SetPES(new(PES))
				}
			}
		}
	}
}
