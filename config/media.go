package config

type Media struct {
	RTP               MediaRTP `yaml:"rtp" json:"rtp"`
	EnabledMediaTypes []string `yaml:"enabled-media-types" json:"enabled-media-types"`
}

func (m *Media) PostModify() (nc any, modified bool, err error) {
	if len(m.EnabledMediaTypes) == 0 {
		m.EnabledMediaTypes = append(m.EnabledMediaTypes, "PS")
	}
	return m, true, nil
}
