package config

import (
	"strconv"
	"strings"
)

type Mysql struct {
	Username string            `yaml:"username" json:"username" default:"cvds"`
	Password string            `yaml:"password" json:"password" default:"cvds2020"`
	Host     string            `yaml:"host" json:"host" default:"localhost"`
	Port     uint16            `yaml:"port" json:"port" default:"3306"`
	DBName   string            `yaml:"db-name" json:"db-name" default:"cvdsdbs"`
	Options  map[string]string `yaml:",inline" json:"options"`
}

func (m *Mysql) DSN() string {
	sb := strings.Builder{}
	if m.Username == "" {
		sb.WriteString("cvds")
	} else {
		sb.WriteString(m.Username)
	}
	if m.Password != "" {
		sb.WriteByte(':')
		sb.WriteString(m.Password)
	}
	sb.WriteString("@(")
	if m.Host == "" {
		sb.WriteString("localhost")
	} else {
		sb.WriteString(m.Host)
	}
	if m.Port != 0 {
		sb.WriteByte(':')
		sb.WriteString(strconv.FormatUint(uint64(m.Port), 10))
	}
	sb.WriteString(")/")
	if m.DBName == "" {
		sb.WriteString("cvdsdbs")
	} else {
		sb.WriteString(m.DBName)
	}
	prefix := "?"
	for key, value := range m.Options {
		sb.WriteString(prefix + key + "=" + value)
		prefix = "&"
	}
	return sb.String()
}
