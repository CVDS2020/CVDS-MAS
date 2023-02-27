package config

import (
	"gitee.com/sy_183/common/def"
)

type Service struct {
	Name        string `yaml:"name" json:"name" default:"CVDSMAS"`
	DisplayName string `yaml:"display-name" json:"display-name"`
	Description string `yaml:"description" json:"description" default:"CVDS Media Access Service"`
}

func (s *Service) PostModify() (nc any, modified bool, err error) {
	def.SetDefaultP(&s.DisplayName, s.Name)
	return s, true, nil
}
