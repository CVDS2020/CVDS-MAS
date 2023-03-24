package db

import (
	"context"
	"fmt"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/log"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
	"runtime"
	"strings"
	"time"
)

const (
	Module     = "db"
	ModuleName = "数据库管理器"
)

type GormLogger struct {
	logger                    log.LoggerProvider
	ignoreRecordNotFoundError bool
	slowThreshold             time.Duration
}

func (l *GormLogger) LogMode(level gormLogger.LogLevel) gormLogger.Interface {
	n := new(GormLogger)
	*n = *l
	return n
}

func (l *GormLogger) Info(ctx context.Context, s string, i ...interface{}) {
	l.logger.Logger().Sugar().Infof(s, i...)
}

func (l *GormLogger) Warn(ctx context.Context, s string, i ...interface{}) {
	l.logger.Logger().Sugar().Warnf(s, i...)
}

func (l *GormLogger) Error(ctx context.Context, s string, i ...interface{}) {
	l.logger.Logger().Sugar().Errorf(s, i...)
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	skip := 2
	for ; skip < 15; skip++ {
		_, file, _, ok := runtime.Caller(skip)
		if ok && (!strings.Contains(file, "gorm.io/gorm") || strings.HasSuffix(file, "_test.go")) {
			break
		}
	}
	logger := l.logger.Logger().WithOptions(log.AddCallerSkip(skip))
	switch {
	case err != nil && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.ignoreRecordNotFoundError):
		sql, rows := fc()
		if rows == -1 {
			logger.Error(err.Error(), log.Duration("花费时间", elapsed), log.String("SQL", sql))
		} else {
			logger.Error(err.Error(), log.Duration("花费时间", elapsed), log.Int64("影响行数", rows), log.String("SQL", sql))
		}
	case elapsed > l.slowThreshold && l.slowThreshold != 0:
		sql, rows := fc()
		slowLog := fmt.Sprintf("慢SQL >= %v", l.slowThreshold)
		if rows == -1 {
			logger.Warn(slowLog, log.Duration("花费时间", elapsed), log.String("SQL", sql))
		} else {
			logger.Warn(slowLog, log.Duration("花费时间", elapsed), log.Int64("影响行数", rows), log.String("SQL", sql))
		}
	default:
		sql, rows := fc()
		if rows == -1 {
			logger.Debug("执行SQL语句成功", log.Duration("花费时间", elapsed), log.String("SQL", sql))
		} else {
			logger.Debug("执行SQL语句成功", log.Duration("花费时间", elapsed), log.Int64("影响行数", rows), log.String("SQL", sql))
		}
	}
}

func WrapGormLogger(logger log.LoggerProvider, ignoreRecordNotFoundError bool, slowThreshold time.Duration) gormLogger.Interface {
	return &GormLogger{
		logger:                    logger,
		ignoreRecordNotFoundError: ignoreRecordNotFoundError,
		slowThreshold:             slowThreshold,
	}
}
