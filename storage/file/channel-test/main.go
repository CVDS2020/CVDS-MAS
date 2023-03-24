package main

import (
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/config"
	dbPkg "gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/storage"
	fileStorage "gitee.com/sy_183/cvds-mas/storage/file"
	"gitee.com/sy_183/cvds-mas/storage/meta"
	"io"
	"time"
)

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

var logger = assert.Must(log.Config{
	Level: log.NewAtomicLevelAt(log.InfoLevel),
	Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
		DisableCaller:     true,
		DisableFunction:   true,
		DisableStacktrace: true,
		EncodeLevel:       log.CapitalColorLevelEncoder,
		EncodeTime:        log.TimeEncoderOfLayout(DefaultTimeLayout),
		EncodeDuration:    log.SecondsDurationEncoder,
	}),
}.Build())

var dbLogger = assert.Must(log.Config{
	Level: log.NewAtomicLevelAt(log.ErrorLevel),
	Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
		DisableCaller:     true,
		DisableFunction:   true,
		DisableStacktrace: true,
		EncodeLevel:       log.CapitalColorLevelEncoder,
		EncodeTime:        log.TimeEncoderOfLayout(DefaultTimeLayout),
		EncodeDuration:    log.SecondsDurationEncoder,
	}),
}.Build())

type Block struct {
	raw []byte
}

func (f Block) WriteTo(w io.Writer) (n int64, err error) {
	_n, err := w.Write(f.raw)
	return int64(_n), err
}

func parseTime(value string) time.Time {
	return assert.Must(time.Parse("2006-01-02 15:04:05", value)).In(time.Local)
}

func main() {
	mysqlConfig := &config.Mysql{
		Username: "root",
		Password: "css66018",
		Host:     "127.0.0.1",
		Port:     3306,
		DBName:   "cvdsrec",
	}
	ch := fileStorage.NewChannel("test",
		fileStorage.WithDirectory("C:\\Users\\suy\\Documents\\Language\\Go\\cvds-mas\\data"),
		storage.WithCover(time.Minute),
		fileStorage.WithFileSize(unit.MeBiByte*10),
		fileStorage.WithCheckDeleteInterval(time.Millisecond*20),
		storage.WithLogger(logger),
		fileStorage.WithMetaManagerConfig(meta.ProvideDBMetaManager, meta.WithDBManager(dbPkg.NewDBManager(nil, mysqlConfig.DSN(), dbPkg.WithTablesInfo(meta.MetaTablesInfos)))),
		fileStorage.WithWriteBufferPoolConfig(3*unit.MeBiByte, pool.StackPoolProvider[*fileStorage.Buffer](2)),
	)
	buf := make([]byte, unit.MeBiByte)
	ch.MetaManager().(*meta.DBMetaManager).DBManager().SetLogger(dbLogger)
	ch.Start()
	for i := 0; i < 10000; i++ {
		start := time.Now()
		time.Sleep(time.Millisecond * 30)
		end := time.Now()
		//if rand.Float64() > 0.3 {
		ch.Write(Block{buf}, uint(len(buf)), start, end, storage.StorageTypeRTP.ID, false)
		//} else {
		//ch.WriteStreamLost(start, end)
		//}
	}
	ch.CloseFile()
	<-ch.ClosedWaiter()
}
