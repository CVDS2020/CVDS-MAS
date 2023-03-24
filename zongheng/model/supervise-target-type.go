package model

const SuperviseTargetTypeTableName = "supervise_target_type"

type SuperviseTargetType struct {
	Id          int32  `gorm:"column:id;primaryKey;not null" json:"id"`
	Type        int32  `gorm:"column:type;not null" json:"type"`
	Name        string `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Description string `gorm:"column:description;type:varchar(255)" json:"description"`
}

var SuperviseTargetTypeModel = new(SuperviseTargetType)

func (t *SuperviseTargetType) Clone() *SuperviseTargetType {
	return clone(t)
}
