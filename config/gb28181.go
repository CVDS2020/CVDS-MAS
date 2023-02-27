package config

type GB28181 struct {
	Proxy   GB28181Proxy    `yaml:"proxy" json:"proxy"`
	Gateway *GB28181Gateway `yaml:"gateway" json:"gateway"`
	DB      DB              `yaml:"db" json:"db"`
}

func (g *GB28181) PreModify() (nc any, modified bool) {
	g.DB.Type = "mysql"
	g.DB.Mysql.DBName = "cvdsrec"
	return g, true
}
