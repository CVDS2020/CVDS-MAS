package model

const TrainTableName = "train"

type Train struct {
	Id          int32   `gorm:"column:id;primaryKey;not null" json:"id"`
	Model       string  `gorm:"column:model;type:varchar(100);not null" json:"model"`
	TrainNo     string  `gorm:"column:trainNo;type:varchar(100);not null" json:"trainNo"`
	Name        string  `gorm:"column:name;type:varchar(255);not null" json:"name"`
	CarriageNum int32   `gorm:"column:carriageNum" json:"carriageNum"`
	Longitude   float32 `gorm:"column:longitude" json:"longitude"`
	Latitude    float32 `gorm:"column:latitude" json:"latitude"`
	Description string  `gorm:"column:description;type:varchar(255)" json:"description"`
}

var TrainModel = new(Train)

func (t *Train) Clone() *Train {
	return clone(t)
}
