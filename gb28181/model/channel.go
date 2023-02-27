package model

const ChannelTableName = "channel"

type Channel struct {
	Id   int64  `gorm:"column:id;primaryKey;not null;autoIncrement;comment:主键ID" json:"id"`
	Name string `gorm:"column:name;type:varchar(255);uniqueIndex:nameUniqueIndex;not null;comment:通道名称" json:"name"`

	ChannelId     string `gorm:"column:channelId;type:varchar(50);uniqueIndex:channelIdUniqueIndex;not null;comment:通道国标ID" json:"channelId"`
	ChannelDomain string `gorm:"column:channelDomain;type:varchar(50);not null;comment:通道域" json:"channelDomain"`
	ChannelIp     string `gorm:"column:channelIp;type:varchar(50);uniqueIndex:channelIpPortUniqueIndex,priority:1;default:NULL;comment:通道IP" json:"channelIp"`
	ChannelPort   int32  `gorm:"column:channelPort;uniqueIndex:channelIpPortUniqueIndex,priority:2;default:NULL;comment:通道端口" json:"channelPort"`
	DisplayName   string `gorm:"column:displayName;not null;comment:通道显示名称" json:"displayName"`

	Gateway       string `gorm:"column:gateway;type:varchar(255);not null;comment:通道网关" json:"gateway"`
	StorageConfig string `gorm:"column:storageConfig;type:varchar(255);not null;comment:通道存储计划" json:"storageConfig"`
	EnableRecord  bool   `gorm:"column:enableRecord;not null;comment:开启音视频录制" json:"enableRecord"`

	Transport  string `gorm:"column:transport;type:varchar(20);not null;comment:SIP传输协议，UDP/TCP" json:"transport"`
	StreamMode string `gorm:"column:streamMode;type:varchar(20);not null;comment:流传输协议，UDP/TCP-ACTIVE/TCP-PASSIVE" json:"streamMode"`
	Charset    string `gorm:"column:charset;type:varchar(20);not null;comment:字符集，UTF-8/GB2312" json:"charset"`

	CreateTime int64 `gorm:"column:createTime;not null;comment:创建时间" json:"createTime"`
	UpdateTime int64 `gorm:"column:updateTime;not null;comment:更新时间" json:"updateTime"`
}

var (
	ChannelModel    = new(Channel)
	ChannelFieldMap = GetFieldMap(ChannelModel)
)
