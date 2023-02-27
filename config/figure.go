package config

type Figure struct {
	Phrase string `yaml:"phrase" json:"phrase" default:"CVDSMAS"`
	Font   string `yaml:"font" json:"font"`
	Color  string `yaml:"color" json:"color"`
	Strict bool   `yaml:"strict" json:"strict"`
}
