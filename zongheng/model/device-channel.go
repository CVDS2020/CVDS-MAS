package model

const DeviceChannelTableName = "device_channel"

type DeviceChannel struct {
	Id        int32  `gorm:"column:id;primaryKey;not null" json:"id"`
	ChannelId string `gorm:"column:channelId;type:varchar(50);not null" json:"channelId"`
	DeviceId  string `gorm:"column:deviceId;type:varchar(50);not null" json:"deviceId"`
	Status    int32  `gorm:"column:status" json:"status"`
}

var DeviceChannelModel = new(DeviceChannel)

func (c *DeviceChannel) Clone() *DeviceChannel {
	return clone(c)
}
