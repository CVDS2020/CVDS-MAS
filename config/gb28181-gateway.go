package config

type GB28181Gateway struct {
	Id             string `yaml:"id" json:"id"`
	SipIp          string `yaml:"sip-ip" json:"sip-ip"`
	SipPort        int    `yaml:"sip-port" json:"sip-port"`
	EnableRegister bool   `yaml:"enable-register" json:"enable-register"`
	Password       string `yaml:"password" json:"password"`
	Expires        int    `yaml:"expires" json:"expires"`
	Transport      string `yaml:"transport" json:"transport"`
}
