package db

import (
	"fmt"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lifecycle/retry"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/utils"
	"gitee.com/sy_183/cvds-mas/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
	"reflect"
	"strings"
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
	runner  *lifecycle.DefaultLifecycle
	started bool

	db          *gorm.DB
	dsn         string
	createTable bool

	ignoreRecordNotFoundError bool
	slowThreshold             time.Duration
	maxOpenConn               int
	maxIdleConn               int
	config                    *gorm.Config

	tablesInfo []TableInfo

	log.AtomicLogger
}

func NewDBManager(db *gorm.DB, dsn string, options ...Option) *DBManager {
	m := &DBManager{
		db:          db,
		dsn:         dsn,
		createTable: true,

		ignoreRecordNotFoundError: true,
		slowThreshold:             time.Second,
		config:                    new(gorm.Config),
	}

	for _, option := range options {
		option.apply(m)
	}
	if m.Logger() == nil {
		m.SetLogger(config.DefaultLogger().Named(ModuleName))
	}
	m.config.Logger = WrapGormLogger(m, m.ignoreRecordNotFoundError, m.slowThreshold)

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
		db, err := gorm.Open(mysql.Open(m.dsn), m.config)
		sqlDB, _ := db.DB()
		if m.maxOpenConn > 0 {
			sqlDB.SetMaxOpenConns(m.maxOpenConn)
		}
		if m.maxIdleConn > 0 {
			sqlDB.SetMaxIdleConns(m.maxIdleConn)
		}
		if err != nil {
			m.Logger().ErrorWith("打开数据连接失败", err)
			return err
		}
		m.db = db
		return nil
	})
}

func (m *DBManager) EnsureTable(tableInfo *TableInfo, interrupter chan struct{}) bool {
	return m.retryDo(interrupter, func() error {
		options := fmt.Sprintf("COMMENT = '%s'", tableInfo.Comment)
		if err := CreateTableIfNotExist(m.Table(tableInfo.Name).Set("gorm:table_options", options).Migrator(), tableInfo.Model); err != nil {
			m.Logger().ErrorWith("确定表创建完成出现错误", err, tableInfo.Fields()...)
			return err
		}
		m.Logger().Info("确定表创建完成", tableInfo.Fields()...)
		return nil
	})
}

func (m *DBManager) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if m.started {
		return nil
	}
	if !m.openDB(interrupter) {
		m.Logger().Warn("打开数据库被中断")
		return lifecycle.NewInterruptedError(ModuleName, "启动")
	}
	if m.createTable {
		for i := range m.tablesInfo {
			if !m.EnsureTable(&m.tablesInfo[i], interrupter) {
				m.Logger().Warn("创建不存在的表被中断", m.tablesInfo[i].Fields()...)
				return lifecycle.NewInterruptedError(ModuleName, "启动")
			}
		}
	}
	m.started = true
	return nil
}

func buildConstraint(constraint *schema.Constraint) (sql string, results []interface{}) {
	sql = "CONSTRAINT ? FOREIGN KEY ? REFERENCES ??"
	if constraint.OnDelete != "" {
		sql += " ON DELETE " + constraint.OnDelete
	}

	if constraint.OnUpdate != "" {
		sql += " ON UPDATE " + constraint.OnUpdate
	}

	var foreignKeys, references []interface{}
	for _, field := range constraint.ForeignKeys {
		foreignKeys = append(foreignKeys, clause.Column{Name: field.DBName})
	}

	for _, field := range constraint.References {
		references = append(references, clause.Column{Name: field.DBName})
	}
	results = append(results, clause.Table{Name: constraint.Name}, foreignKeys, clause.Table{Name: constraint.ReferenceSchema.Table}, references)
	return
}

func CreateTableIfNotExist(m gorm.Migrator, dst any) error {
	var mig migrator.Migrator
	switch m := m.(type) {
	case mysql.Migrator:
		mig = m.Migrator
	case migrator.Migrator:
		mig = m
	default:
		if !m.HasTable(dst) {
			return m.CreateTable(dst)
		}
	}
	tx := mig.DB.Session(&gorm.Session{})
	return mig.RunWithValue(dst, func(stmt *gorm.Statement) (err error) {
		var (
			createTableSQL          = "CREATE TABLE IF NOT EXISTS ? ("
			values                  = []interface{}{mig.CurrentTable(stmt)}
			hasPrimaryKeyInDataType bool
		)

		for _, dbName := range stmt.Schema.DBNames {
			field := stmt.Schema.FieldsByDBName[dbName]
			if !field.IgnoreMigration {
				createTableSQL += "? ?"
				hasPrimaryKeyInDataType = hasPrimaryKeyInDataType || strings.Contains(strings.ToUpper(string(field.DataType)), "PRIMARY KEY")
				values = append(values, clause.Column{Name: dbName}, mig.DB.Migrator().FullDataTypeOf(field))
				createTableSQL += ","
			}
		}

		if !hasPrimaryKeyInDataType && len(stmt.Schema.PrimaryFields) > 0 {
			createTableSQL += "PRIMARY KEY ?,"
			var primaryKeys []interface{}
			for _, field := range stmt.Schema.PrimaryFields {
				primaryKeys = append(primaryKeys, clause.Column{Name: field.DBName})
			}

			values = append(values, primaryKeys)
		}

		for _, idx := range stmt.Schema.ParseIndexes() {
			if mig.CreateIndexAfterCreateTable {
				defer func(value interface{}, name string) {
					if err == nil {
						err = tx.Migrator().CreateIndex(value, name)
					}
				}(dst, idx.Name)
			} else {
				if idx.Class != "" {
					createTableSQL += idx.Class + " "
				}
				createTableSQL += "INDEX ? ?"

				if idx.Comment != "" {
					createTableSQL += fmt.Sprintf(" COMMENT '%s'", idx.Comment)
				}

				if idx.Option != "" {
					createTableSQL += " " + idx.Option
				}

				createTableSQL += ","
				values = append(values, clause.Column{Name: idx.Name}, tx.Migrator().(migrator.BuildIndexOptionsInterface).BuildIndexOptions(idx.Fields, stmt))
			}
		}

		for _, rel := range stmt.Schema.Relationships.Relations {
			if !mig.DB.DisableForeignKeyConstraintWhenMigrating {
				if constraint := rel.ParseConstraint(); constraint != nil {
					if constraint.Schema == stmt.Schema {
						sql, vars := buildConstraint(constraint)
						createTableSQL += sql + ","
						values = append(values, vars...)
					}
				}
			}
		}

		for _, chk := range stmt.Schema.ParseCheckConstraints() {
			createTableSQL += "CONSTRAINT ? CHECK (?),"
			values = append(values, clause.Column{Name: chk.Name}, clause.Expr{SQL: chk.Constraint})
		}

		createTableSQL = strings.TrimSuffix(createTableSQL, ",")

		createTableSQL += ")"

		if tableOption, ok := mig.DB.Get("gorm:table_options"); ok {
			createTableSQL += fmt.Sprint(tableOption)
		}

		tx = tx.Exec(createTableSQL, values...)
		return tx.Error
	})
}
