package meta

import (
	"gitee.com/sy_183/common/log"
	dbPkg "gitee.com/sy_183/cvds-mas/db"
)

type Option interface {
	Apply(m Manager) any
}

type OptionFunc func(m Manager) any

func (f OptionFunc) Apply(m Manager) any {
	return f(m)
}

type OptionCustom func(m Manager)

func (f OptionCustom) Apply(m Manager) any {
	f(m)
	return nil
}

//func WithDB(db *gorm.DB) Option {
//	return OptionCustom(func(m Manager) {
//		if dbm, is := m.(*DBMetaManager); is {
//			dbm.dbManager = dbPkg.NewDBManager(db, "", metaTablesInfos, true, nil)
//		}
//	})
//}
//
//func WithDSN(dsn string) Option {
//	return OptionCustom(func(m Manager) {
//		if dbm, is := m.(*DBMetaManager); is {
//			dbm.dbManager = dbPkg.NewDBManager(nil, dsn, metaTablesInfos, true, nil)
//		}
//	})
//}

func WithDBManager(dbManager *dbPkg.DBManager) Option {
	return OptionCustom(func(m Manager) {
		if dbm, is := m.(*DBMetaManager); is {
			dbm.dbManager = dbManager
		}
	})
}

func WithEarliestIndexesCacheSize(size int) Option {
	type earliestIndexesCacheSizeSetter interface {
		setEarliestIndexesCacheSize(size int)
	}
	return OptionCustom(func(m Manager) {
		if setter, is := m.(earliestIndexesCacheSizeSetter); is {
			setter.setEarliestIndexesCacheSize(size)
		}
	})
}

func WithNeedCreatedIndexesCacheSize(size int) Option {
	type needCreatedIndexesCacheSizeSetter interface {
		setNeedCreatedIndexesCacheSize(size int)
	}
	return OptionCustom(func(m Manager) {
		if setter, is := m.(needCreatedIndexesCacheSizeSetter); is {
			setter.setNeedCreatedIndexesCacheSize(size)
		}
	})
}

func WithMaxFiles(maxFiles int) Option {
	type maxFilesSetter interface {
		setMaxFiles(maxFiles int)
	}
	return OptionCustom(func(m Manager) {
		if setter, is := m.(maxFilesSetter); is {
			setter.setMaxFiles(maxFiles)
		}
	})
}

func WithLogger(logger *log.Logger) Option {
	return OptionCustom(func(m Manager) {
		m.SetLogger(logger)
	})
}
