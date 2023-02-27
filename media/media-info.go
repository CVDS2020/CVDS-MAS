package media

import (
	"gitee.com/sy_183/common/log"
	"net"
)

type MediaInfo struct {
	Transport  string
	LocalIP    net.IP
	LocalPort  uint16
	RemoteIP   net.IP
	RemotePort uint16
	SSRC       *uint32
}

func getAddr(ip net.IP, port uint16, transport string) net.Addr {
	switch transport {
	case "tcp":
		return &net.TCPAddr{
			IP:   ip,
			Port: int(port),
		}
	case "udp":
		return &net.UDPAddr{
			IP:   ip,
			Port: int(port),
		}
	default:
		return nil
	}
}

func (m *MediaInfo) LocalAddr(transport string) net.Addr {
	return getAddr(m.LocalIP, m.LocalPort, transport)
}

func (m *MediaInfo) RemoteAddr(transport string) net.Addr {
	return getAddr(m.RemoteIP, m.RemotePort, transport)
}

func (m *MediaInfo) MarshalLogObject(encoder log.ObjectEncoder) error {
	if m.Transport == "" {
		encoder.AddString("transport", "UNKNOWN")
	} else {
		encoder.AddString("transport", m.Transport)
	}
	if m.LocalIP != nil {
		encoder.AddString("local-host", m.LocalIP.String())
	}
	if m.LocalPort != 0 {
		encoder.AddUint16("local-port", m.LocalPort)
	}
	if m.RemoteIP != nil {
		encoder.AddString("remote-host", m.RemoteIP.String())
	}
	if m.RemotePort != 0 {
		encoder.AddUint16("remote-port", m.RemotePort)
	}
	if m.SSRC != nil {
		encoder.AddUint32("ssrc", *m.SSRC)
	}
	return nil
}
