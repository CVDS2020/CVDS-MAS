package config

import (
	"time"
)

type Mongo struct {
	Database string   `yaml:"database" json:"database" default:"cvds2020"`
	Hosts    []string `yaml:"hosts" json:"hosts"`
	Auth     struct {
		AuthMechanism           string            `yaml:"auth-mechanism" json:"auth-mechanism" default:"SCRAM-SHA-1"`
		AuthMechanismProperties map[string]string `yaml:"auth-mechanism-properties" json:"auth-mechanism-properties"`
		AuthSource              string            `yaml:"auth-source" json:"auth-source"`
		Username                string            `yaml:"username" json:"username" default:"cvds"`
		Password                string            `yaml:"password" json:"password" default:"cvds2020"`
		PasswordSet             bool              `yaml:"password-set" json:"password-set" default:"true"`
	} `yaml:"auth" json:"auth"`
	ReplicaSet               *string        `yaml:"replica-set" json:"replica-set"`
	AppName                  *string        `yaml:"app-name" json:"app-name"`
	Compressors              []string       `yaml:"compressors" json:"compressors"`
	ConnectTimeout           *time.Duration `yaml:"connect-timeout" json:"connect-timeout"`
	Direct                   *bool          `yaml:"direct" json:"direct"`
	HeartbeatInterval        *time.Duration `yaml:"heartbeat-interval" json:"heartbeat-interval"`
	LoadBalanced             *bool          `yaml:"load-balanced" json:"load-balanced"`
	LocalThreshold           *time.Duration `yaml:"local-threshold" json:"local-threshold"`
	MaxConnIdleTime          *time.Duration `yaml:"max-conn-idle-time" json:"max-conn-idle-time"`
	MaxPoolSize              *uint64        `yaml:"max-pool-size" json:"max-pool-size"`
	MinPoolSize              *uint64        `yaml:"min-pool-size" json:"min-pool-size"`
	MaxConnecting            *uint64        `yaml:"max-connecting" json:"max-connecting"`
	RetryWrites              *bool          `yaml:"retry-writes" json:"retry-writes"`
	RetryReads               *bool          `yaml:"retry-reads" json:"retry-reads"`
	ServerSelectionTimeout   *time.Duration `yaml:"server-selection-timeout" json:"server-selection-timeout"`
	SocketTimeout            *time.Duration `yaml:"socket-timeout" json:"socket-timeout"`
	ZlibLevel                *int           `yaml:"zlib-level" json:"zlib-level"`
	ZstdLevel                *int           `yaml:"zstd-level" json:"zstd-level"`
	DisableOCSPEndpointCheck *bool          `yaml:"disable-ocsp-endpoint-check" json:"disable-ocsp-endpoint-check"`
	SRVMaxHosts              *int           `yaml:"srv-max-hosts" json:"srv-max-hosts"`
	SRVServiceName           *string        `yaml:"srv-service-name" yaml:"srv-service-name"`
}
