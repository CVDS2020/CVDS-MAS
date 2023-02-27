package db

import (
	"fmt"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lifecycle/retry"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/utils"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"reflect"
	"time"
)

type TableInfo struct {
	Name    string
	Comment string
	Model   any
}

func (t *TableInfo) Fields() []log.Field {
	return []log.Field{
		log.String("表名", t.Name),
		log.String("描述", t.Comment),
		log.String("模型", reflect.TypeOf(t.Model).String()),
	}
}

type DBManager struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	db         *gorm.DB
	dsn        string
	tablesInfo []TableInfo

	log.AtomicLogger
}

func NewDBManager(db *gorm.DB, dsn string, tablesInfo []TableInfo, logger *log.Logger) *DBManager {
	m := &DBManager{
		db:         db,
		dsn:        dsn,
		tablesInfo: tablesInfo,
	}
	if logger == nil {
		logger = Logger()
	}
	m.SetLogger(logger)
	m.runner = lifecycle.NewWithInterruptedRun(m.start, nil)
	m.Lifecycle = m.runner
	return m
}

func (m *DBManager) DB() *gorm.DB {
	return m.db
}

func (m *DBManager) DSN() string {
	return m.dsn
}

func (m *DBManager) Table(name string) *gorm.DB {
	return m.TableWith(m.db, name)
}

func (m *DBManager) TableWith(db *gorm.DB, name string) *gorm.DB {
	return db.Scopes(func(db *gorm.DB) (tx *gorm.DB) {
		return db.Table(name)
	})
}

func (m *DBManager) retryDo(interrupter chan struct{}, fn func() error) bool {
	if err := retry.MakeRetry(retry.Retry{
		Do:          fn,
		MaxRetry:    -1,
		Interrupter: interrupter,
	}).Todo(); err == retry.InterruptedError {
		return false
	}
	if _, interrupted := utils.ChanTryPop(interrupter); interrupted {
		return false
	}
	return true
}

func (m *DBManager) openDB(interrupter chan struct{}) bool {
	if m.db != nil {
		return true
	}
	return m.retryDo(interrupter, func() error {
		db, err := gorm.Open(mysql.Open(m.dsn), &gorm.Config{
			Logger: WrapGormLogger(m.Logger(), true, time.Millisecond*200),
		})
		if err != nil {
			m.Logger().ErrorWith("打开数据连接失败", err)
			return err
		}
		m.db = db
		return nil
	})
}

func (m *DBManager) ensureTable(tableInfo *TableInfo, interrupter chan struct{}) bool {
	return m.retryDo(interrupter, func() error {
		if !m.Table(tableInfo.Name).Migrator().HasTable(tableInfo.Name) {
			options := fmt.Sprintf("COMMENT = '%s'", tableInfo.Comment)
			m.Logger().Info("设备信息表未创建，创建此表")
			if err := m.Table(tableInfo.Name).Set("gorm:table_options", options).Migrator().CreateTable(tableInfo.Model); err != nil {
				m.Logger().ErrorWith("创建表失败", err, tableInfo.Fields()...)
				return err
			}
			m.Logger().Info("创建表成功", tableInfo.Fields()...)
		}
		return nil
	})
}

func (m *DBManager) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if !m.openDB(interrupter) {
		m.Logger().Warn("打开数据库被中断")
		return lifecycle.NewInterruptedError(ModuleName, "启动")
	}
	for i := range m.tablesInfo {
		if !m.ensureTable(&m.tablesInfo[i], interrupter) {
			m.Logger().Warn("创建不存在的表被中断", m.tablesInfo[i].Fields()...)
			return lifecycle.NewInterruptedError(ModuleName, "启动")
		}
	}
	return nil
}
