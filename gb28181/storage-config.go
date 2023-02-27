package gb28181

import (
	"fmt"
	"gitee.com/sy_183/common/lifecycle"
	taskPkg "gitee.com/sy_183/common/lifecycle/task"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/uns"
	"gitee.com/sy_183/cvds-mas/db"
	gbErrors "gitee.com/sy_183/cvds-mas/gb28181/errors"
	modelPkg "gitee.com/sy_183/cvds-mas/gb28181/model"
	"gorm.io/gorm"
	"sync/atomic"
	"unsafe"
)

type StorageConfig struct {
	name  string
	model atomic.Pointer[modelPkg.StorageConfig]

	manager   *StorageConfigManager
	dbManager *db.DBManager
	dbTask    *taskPkg.TaskExecutor
	dbAdded   atomic.Bool
	dbDelete  atomic.Bool

	log.AtomicLogger
}

func NewStorageConfig(manager *StorageConfigManager, model *modelPkg.StorageConfig, create bool, createFuture lifecycle.Future[error]) *StorageConfig {
	sc := &StorageConfig{
		name:      model.Name,
		manager:   manager,
		dbManager: manager.DBManager(),
		dbTask:    taskPkg.NewTaskExecutor(1),
	}
	sc.model.Store(model)
	sc.SetLogger(manager.Logger().Named(sc.DisplayName()))
	sc.dbTask.Start()
	if create {
		sc.dbTask.Async(taskPkg.Func(func() {
			if res := sc.dbManager.Table(modelPkg.StorageConfigTableName).Create(model); res.Error != nil {
				createFuture.Complete(sc.Logger().ErrorWith("向数据库中添加存储配置失败", res.Error))
				return
			}
			sc.dbAdded.Store(true)
			createFuture.Complete(nil)
		}))
	} else {
		sc.dbAdded.Store(true)
	}
	return sc
}

func (c *StorageConfig) DisplayName() string {
	return fmt.Sprintf("存储配置服务(%s)", c.name)
}

func (c *StorageConfig) Model() *modelPkg.StorageConfig {
	return c.model.Load()
}

func (c *StorageConfig) Update(model *modelPkg.StorageConfig, fields []string, future lifecycle.Future[error]) error {
	if !c.dbAdded.Load() || c.dbDelete.Load() {
		return gbErrors.StorageConfigNotAvailableError
	}

	oldModel := c.Model()
	newModel := oldModel.Clone()
	for _, field := range fields {
		if structField, has := modelPkg.StorageConfigFieldMap[field]; has {
			uns.Copy(
				unsafe.Add(unsafe.Pointer(newModel), structField.Offset),
				unsafe.Add(unsafe.Pointer(model), structField.Offset),
				int(structField.Type.Size()),
			)
		}
	}
	if newModel.Name != oldModel.Name {
		return c.Logger().ErrorWith("更新存储配置失败", gbErrors.StorageConfigModifiedError)
	}

	if future == nil {
		future = lifecycle.NopFuture[error]{}
	}
	if ok, err := c.dbTask.Try(taskPkg.Func(func() {
		if c.dbDelete.Load() {
			future.Complete(c.Logger().ErrorWith("更新存储配置失败", gbErrors.StorageConfigNotAvailableError))
			return
		}
		future.Complete(c.dbManager.DB().Transaction(func(tx *gorm.DB) (err error) {
			res := c.dbManager.TableWith(tx, modelPkg.StorageConfigTableName).Select(fields).
				Where("name = ?", newModel.Name).Updates(newModel)
			if res.Error != nil {
				return c.Logger().ErrorWith("更新数据库中的存储配置失败", res.Error)
			}
			c.model.Store(newModel)
			return nil
		}))
	})); err != nil {
		return c.Logger().ErrorWith("更新存储配置失败", gbErrors.StorageConfigNotAvailableError)
	} else if !ok {
		return c.Logger().ErrorWith("更新存储配置失败", gbErrors.StorageConfigDBTaskFullError)
	}
	return nil
}

func (c *StorageConfig) Delete(future lifecycle.Future[error]) error {
	if !c.dbAdded.Load() {
		return gbErrors.StorageConfigNotAvailableError
	}
	if !c.dbDelete.CompareAndSwap(false, true) {
		return gbErrors.StorageConfigNotAvailableError
	}

	if ok, err := c.dbTask.Try(taskPkg.Func(func() {
		if res := c.dbManager.Table(modelPkg.StorageConfigTableName).Delete(c.Model()); res.Error != nil {
			c.dbDelete.Store(false)
			future.Complete(c.Logger().ErrorWith("从数据库中删除存储配置失败", res.Error))
			return
		}
		future.Complete(nil)
	})); err != nil {
		return c.Logger().ErrorWith("删除存储配置失败", gbErrors.StorageConfigNotAvailableError)
	} else if !ok {
		return c.Logger().ErrorWith("删除存储配置失败", gbErrors.StorageConfigDBTaskFullError)
	}
	return nil
}
