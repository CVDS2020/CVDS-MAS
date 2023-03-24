package model

const SuperviseTargetTableName = "supervise_target"

type SuperviseTarget struct {
	Id          int32  `gorm:"column:id;primaryKey;not null" json:"id"`
	Name        string `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Type        int32  `gorm:"column:type;not null" json:"type"`
	CarriageNo  int32  `gorm:"column:carriageNo" json:"carriageNo"`
	Address     string `gorm:"column:address;type:varchar(255)" json:"address"`
	Status      int32  `gorm:"column:status" json:"status"`
	StatusText  string `gorm:"column:statusText;type:varchar(255)" json:"statusText"`
	Description string `gorm:"column:description;type:varchar(255)" json:"description"`
}

var SuperviseTargetModel = new(SuperviseTarget)

func (t *SuperviseTarget) Clone() *SuperviseTarget {
	return clone(t)
}
