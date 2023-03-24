package gb28181

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/gb28181/model"
)

const (
	DBManagerModule     = Module + ".db"
	DBManagerModuleName = "国标数据库管理器"
)

var GetDBManager = component.NewPointer(func() *db.DBManager {
	dbManager := db.NewDBManager(nil, config.GB28181DBConfig().Mysql.DSN(), db.WithTablesInfo([]db.TableInfo{
		{Name: model.ChannelTableName, Comment: "通道表", Model: model.ChannelModel},
		{Name: model.StreamTableName, Comment: "通道流表", Model: model.StreamModel},
		{Name: model.GatewayTableName, Comment: "网关表", Model: model.GatewayModel},
		{Name: model.StorageConfigTableName, Comment: "存储配置表", Model: model.StorageConfigModel},
	}))
	config.InitModuleLogger(dbManager, DBManagerModule, DBManagerModuleName)
	config.RegisterLoggerConfigReloadedCallback(dbManager, DBManagerModule, DBManagerModuleName)
	return dbManager
}).Get
