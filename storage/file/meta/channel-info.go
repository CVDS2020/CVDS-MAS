package meta

import (
	"gitee.com/sy_183/common/log"
	"time"
)

type ChannelInfo struct {
	ID             uint64 `gorm:"column:id;primaryKey;not null;autoIncrement"`
	Name           string `gorm:"column:name;uniqueIndex:name_uindex;not null;comment:通道名称"`
	Location       string `gorm:"column:location;not null;default:UTC;comment:通道时区名称"`
	LocationOffset int64  `gorm:"column:location_offset;not null;default:0;comment:通道时区偏移，单位ms"`
	TimeOffset     int64  `gorm:"column:time_offset;not null;default:0;comment:通道时间偏移，单位ms"`
}

func (c *ChannelInfo) Clone() *ChannelInfo {
	n := new(ChannelInfo)
	*n = *c
	return n
}

func (c *ChannelInfo) TableName() string {
	return "channel_info"
}

func (c *ChannelInfo) MarshalLogObject(encoder log.ObjectEncoder) error {
	encoder.AddUint64("id", c.ID)
	encoder.AddString("name", c.Name)
	encoder.AddString("location", c.Location)
	encoder.AddDuration("location_offset", time.Duration(c.LocationOffset)*time.Millisecond)
	encoder.AddDuration("time_offset", time.Duration(c.TimeOffset)*time.Millisecond)
	return nil
}
