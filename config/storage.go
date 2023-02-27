package config

type Storage struct {
	Type string      `yaml:"type" json:"type" default:"file"`
	File StorageFile `yaml:"file" json:"file"`
}
