package model

const DeviceTableName = "device"

type Device struct {
	Id                  int32  `gorm:"column:id;primaryKey;not null" json:"id"`
	DeviceId            string `gorm:"column:deviceId;type:varchar(50);not null" json:"deviceId"`
	Name                string `gorm:"column:name;type:varchar(255)" json:"name"`
	Manufacturer        string `gorm:"column:manufacturer;type:varchar(255)" json:"manufacturer"`
	Model               string `gorm:"column:model;type:varchar(255)" json:"model"`
	Firmware            string `gorm:"column:firmware;type:varchar(255)" json:"firmware"`
	Ip                  string `gorm:"column:ip;type:varchar(50);not null" json:"ip"`
	Port                int32  `gorm:"column:port" json:"port"`
	SuperviseTargetType int32  `gorm:"column:superviseTargetType" json:"superviseTargetType"`
	SuperviseTargetId   int32  `gorm:"column:superviseTargetId" json:"superviseTargetId"`
	Position            string `gorm:"column:position;type:varchar(255)" json:"position"`
	CarriageNo          int32  `gorm:"column:carriageNo" json:"carriageNo"`
}

var DeviceModel = new(Device)

func (d *Device) Clone() *Device {
	return clone(d)
}
