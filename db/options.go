package db

import "time"

type Option interface {
	apply(manager *DBManager)
}

type optionFunc func(manager *DBManager)

func (f optionFunc) apply(manager *DBManager) {
	f(manager)
}

func WithTablesInfo(tablesInfo []TableInfo) Option {
	return optionFunc(func(manager *DBManager) {
		manager.tablesInfo = tablesInfo
	})
}

func WithCreateTable(createTable bool) Option {
	return optionFunc(func(manager *DBManager) {
		manager.createTable = createTable
	})
}

func WithIgnoreRecordNotFoundError(ignore bool) Option {
	return optionFunc(func(manager *DBManager) {
		manager.ignoreRecordNotFoundError = ignore
	})
}

func WithSlowThreshold(threshold time.Duration) Option {
	return optionFunc(func(manager *DBManager) {
		manager.slowThreshold = threshold
	})
}

func WithSkipDefaultTransaction(skip bool) Option {
	return optionFunc(func(manager *DBManager) {
		manager.config.SkipDefaultTransaction = skip
	})
}

func WithPrepareStmt(prepareStmt bool) Option {
	return optionFunc(func(manager *DBManager) {
		manager.config.PrepareStmt = prepareStmt
	})
}

func WithMaxOpenConn(n int) Option {
	return optionFunc(func(manager *DBManager) {
		manager.maxOpenConn = n
	})
}

func WithMaxIdleConn(n int) Option {
	return optionFunc(func(manager *DBManager) {
		manager.maxIdleConn = n
	})
}
