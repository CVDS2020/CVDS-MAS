package config

import (
	"gitee.com/sy_183/common/unit"
	"net"
	"time"
)

type MediaRTP struct {
	LocalIP      string `yaml:"local-ip" json:"local-ip"`
	localIP      net.IP
	ListenAddr   string `yaml:"listen-addr" json:"listen-addr" default:"0.0.0.0"`
	listenIPAddr *net.IPAddr
	Port         uint16 `yaml:"port" json:"port" default:"5004"`
	Ports        []struct {
		RTP  uint16 `yaml:"rtp" json:"rtp"`
		RTCP uint16 `yaml:"rtcp" json:"rtcp"`
	} `yaml:"ports" json:"ports"`
	PortRange struct {
		Start    uint16   `yaml:"start" json:"start" default:"5004"`
		End      uint16   `yaml:"end" json:"end" default:"5204"`
		Excludes []uint16 `yaml:"excludes" json:"excludes"`
	} `yaml:"port-range" json:"port-range"`
	Socket struct {
		ReadBuffer      int           `yaml:"read-buffer" json:"read-buffer"`
		WriteBuffer     int           `yaml:"write-buffer" json:"write-buffer"`
		Keepalive       bool          `yaml:"keepalive" json:"keepalive"`
		KeepalivePeriod time.Duration `yaml:"keepalive-period" json:"keepalive-period"`
		DisableNoDelay  bool          `yaml:"disable-no-delay" json:"disable-no-delay"`
	} `yaml:"socket" json:"socket"`
	Buffer           unit.Size     `yaml:"buffer" json:"buffer" default:"256KiB"`
	BufferReverse    unit.Size     `yaml:"buffer-reverse" json:"buffer-reverse" default:"2048"`
	BufferPoolType   string        `yaml:"buffer-pool-type" json:"buffer-pool-type" default:"slice"`
	BufferCount      uint          `yaml:"buffer-count" json:"buffer-count" default:"8"`
	RetryInterval    time.Duration `yaml:"retry-interval" json:"retry-interval" default:"1s"`
	OrderCache       uint16        `yaml:"order-cache" json:"order-cache"`
	FrameWriteBuffer unit.Size     `yaml:"frame-write-cache" json:"frame-write-cache"`
}

func (m *MediaRTP) PostModify() (nc any, modified bool, err error) {
	addr, err := net.ResolveIPAddr("ip", m.ListenAddr)
	if err != nil {
		return m, false, err
	}
	m.listenIPAddr = addr
	if m.LocalIP != "" {
		addr, err = net.ResolveIPAddr("ip", m.LocalIP)
		if err != nil {
			return m, false, err
		}
		m.localIP = addr.IP
	}
	return m, true, nil
}

func (m *MediaRTP) ListenIPAddr() *net.IPAddr {
	return m.listenIPAddr
}

func (m *MediaRTP) GetLocalIP() net.IP {
	return m.localIP
}
