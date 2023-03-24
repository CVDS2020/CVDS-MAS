package gb28181

import (
	"errors"
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/db"
	gbErrors "gitee.com/sy_183/cvds-mas/gb28181/errors"
	modelPkg "gitee.com/sy_183/cvds-mas/gb28181/model"
	"gorm.io/gorm"
	"time"
)

const (
	StorageConfigManagerModule     = Module + ".storage-config-manager"
	StorageConfigManagerModuleName = "存储配置管理器"
)

type StorageConfigManager struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	dbManager *db.DBManager

	log.AtomicLogger
}

func NewStorageConfigManager() *StorageConfigManager {
	m := &StorageConfigManager{
		dbManager: GetDBManager(),
	}
	config.InitModuleLogger(m, StorageConfigManagerModule, StorageConfigManagerModuleName)
	config.RegisterLoggerConfigReloadedCallback(m, StorageConfigManagerModule, StorageConfigManagerModuleName)
	m.runner = lifecycle.NewWithInterruptedRun(nil, lifecycle.InterrupterHoldRun)
	m.Lifecycle = m.runner
	return m
}

func (m *StorageConfigManager) DBManager() *db.DBManager {
	return m.dbManager
}

func (m *StorageConfigManager) GetStorageConfig(name string) (*modelPkg.StorageConfig, error) {
	model := new(modelPkg.StorageConfig)
	if res := m.dbManager.Table(modelPkg.StorageConfigTableName).Where("name = ?", name).First(model); res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, m.Logger().ErrorWith("从数据库中获取存储配置失败", res.Error)
	}
	return model, nil
}

func (m *StorageConfigManager) AddStorageConfig(model *modelPkg.StorageConfig) error {
	model = model.Clone()
	model.Id = 0
	if model.Name == "" {
		return &gbErrors.ArgumentMissingError{Arguments: []string{"name"}}
	}
	if model.Cover <= 0 {
		return &gbErrors.InvalidArgumentError{Argument: "cover", Err: fmt.Errorf("错误的覆盖周期%d", model.Cover)}
	}
	model.CreateTime = time.Now().UnixMilli()
	model.UpdateTime = time.Now().UnixMilli()
	return lock.RLockGet(m.runner, func() error {
		if !m.runner.Running() {
			return m.Logger().ErrorWith("添加存储配置失败", gbErrors.StorageConfigManagerNotAvailableError)
		}
		if res := m.dbManager.Table(modelPkg.StorageConfigTableName).Create(model); res.Error != nil {
			return m.Logger().ErrorWith("添加存储配置失败", res.Error)
		}
		return nil
	})
}

func (m *StorageConfigManager) DeleteStorageConfig(name string) error {
	if res := m.dbManager.Table(modelPkg.ChannelTableName).Where("storageConfig = ?", name).First(&modelPkg.Channel{}); res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			if res := m.dbManager.Table(modelPkg.StorageConfigTableName).Where("name = ?", name).Delete(&modelPkg.StorageConfig{}); res.Error != nil {
				return m.Logger().ErrorWith("删除存储配置失败", res.Error)
			}
			return nil
		}
		return m.Logger().ErrorWith("删除存储配置失败，由于获取存储通道对应的通道失败", res.Error)
	}
	return m.Logger().ErrorWith("删除存储配置失败", gbErrors.StorageConfigRelateChannelError)
}

var GetStorageConfigManager = component.NewPointer(NewStorageConfigManager).Get
