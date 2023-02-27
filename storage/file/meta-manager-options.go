package file

import (
	"gitee.com/sy_183/common/log"
	dbPkg "gitee.com/sy_183/cvds-mas/db"
	"gorm.io/gorm"
)

type MetaManagerOption interface {
	Apply(m MetaManager) any
}

type MetaManagerOptionFunc func(m MetaManager) any

func (f MetaManagerOptionFunc) Apply(m MetaManager) any {
	return f(m)
}

type MetaManagerOptionCustom func(m MetaManager)

func (f MetaManagerOptionCustom) Apply(m MetaManager) any {
	f(m)
	return nil
}

func WithDBMetaManagerDB(db *gorm.DB) MetaManagerOption {
	return MetaManagerOptionCustom(func(m MetaManager) {
		if dbm, is := m.(*dbMetaManager); is {
			dbm.dbManager = dbPkg.NewDBManager(db, "", metaTablesInfos, nil)
		}
	})
}

func WithDBMetaManagerDSN(dsn string) MetaManagerOption {
	return MetaManagerOptionCustom(func(m MetaManager) {
		if dbm, is := m.(*dbMetaManager); is {
			dbm.dbManager = dbPkg.NewDBManager(nil, dsn, metaTablesInfos, nil)
		}
	})
}

func WithMetaManagerEarliestIndexesCacheSize(size int) MetaManagerOption {
	return MetaManagerOptionCustom(func(m MetaManager) {
		if setter, is := m.(interface{ setEarliestIndexesCacheSize(size int) }); is {
			setter.setEarliestIndexesCacheSize(size)
		}
	})
}

func WithMetaManagerNeedCreatedIndexesCacheSize(size int) MetaManagerOption {
	return MetaManagerOptionCustom(func(m MetaManager) {
		if setter, is := m.(interface{ setNeedCreatedIndexesCacheSize(size int) }); is {
			setter.setNeedCreatedIndexesCacheSize(size)
		}
	})
}

func WithMetaManagerMaxFiles(maxFiles int) MetaManagerOption {
	return MetaManagerOptionCustom(func(m MetaManager) {
		if setter, is := m.(interface{ setMaxFiles(maxFiles int) }); is {
			setter.setMaxFiles(maxFiles)
		}
	})
}

func WithDBMetaManagerLogger(logger *log.Logger) MetaManagerOption {
	return MetaManagerOptionCustom(func(m MetaManager) {
		m.SetLogger(logger)
	})
}
