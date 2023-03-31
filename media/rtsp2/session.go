package rtsp2

import mediaChannel "gitee.com/sy_183/cvds-mas/media/channel"

type Session struct {
	sessionId        string
	rtpPusher        *mediaChannel.RtpPusher
	historyRtpPusher *mediaChannel.HistoryRtpPusher
}

func (s *Session) play() {
	if s.rtpPusher != nil {
		s.rtpPusher.StartPush()
	} else {
		s.historyRtpPusher.StartPush()
	}
}

func (s *Session) pause() {
	if s.rtpPusher != nil {
		s.rtpPusher.Pause()
	} else {
		s.historyRtpPusher.Pause()
	}
}

func (s *Session) close() {
	if s.rtpPusher != nil {
		s.rtpPusher.Close(nil)
	} else {
		s.historyRtpPusher.Close(nil)
	}
}
