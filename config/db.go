package config

type DB struct {
	Type  string `yaml:"type" json:"type" default:"mysql"`
	Mongo Mongo  `yaml:"mongo" json:"mongo"`
	Mysql Mysql  `yaml:"mysql" json:"mysql"`
}
