package zongheng

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/zongheng/model"
)

const (
	DBManagerModule     = "zongheng.db"
	DBManagerModuleName = "纵横数据库管理器"
)

var GetDBManager = component.NewPointer(func() *db.DBManager {
	dbManager := db.NewDBManager(nil, config.ZongHengDBConfig().Mysql.DSN(), db.WithTablesInfo([]db.TableInfo{
		{Name: model.DeviceTableName, Model: model.DeviceModel},
		{Name: model.DeviceChannelTableName, Model: model.DeviceChannelModel},
		{Name: model.SuperviseTargetTableName, Model: model.SuperviseTargetModel},
		{Name: model.SuperviseTargetTypeTableName, Model: model.SuperviseTargetTypeModel},
		{Name: model.TrainTableName, Model: model.TrainModel},
	}), db.WithCreateTable(false))
	config.InitModuleLogger(dbManager, DBManagerModule, DBManagerModuleName)
	config.RegisterLoggerConfigReloadedCallback(dbManager, DBManagerModule, DBManagerModuleName)
	return dbManager
}).Get
