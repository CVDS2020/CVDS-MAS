package model

const StorageConfigTableName = "storage_config"

type StorageConfig struct {
	Id    int64  `gorm:"column:id;primaryKey;not null;autoIncrement;comment:主键ID" json:"id"`
	Name  string `gorm:"column:name;type:varchar(255);uniqueIndex:nameUniqueIndex;not null;comment:名称" json:"name"`
	Cover int64  `gorm:"column:cover;not null;comment:覆盖周期，单位：分钟" json:"cover"`

	CreateTime int64 `gorm:"column:createTime;not null;comment:创建时间" json:"createTime" json:"createTime"`
	UpdateTime int64 `gorm:"column:updateTime;not null;comment:更新时间" json:"updateTime" json:"updateTime"`
}

var (
	StorageConfigModel    = new(StorageConfig)
	StorageConfigFieldMap = GetFieldMap(StorageConfigModel)
)

func (c *StorageConfig) Clone() *StorageConfig {
	n := new(StorageConfig)
	*n = *c
	return n
}
