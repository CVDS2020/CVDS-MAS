package main

import (
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/storage"
	filePkg "gitee.com/sy_183/cvds-mas/storage/file"
	"io"
	"time"
)

type TestFrame struct {
	*pool.Data
	Start time.Time
	End   time.Time
}

func (f *TestFrame) WriteTo(w io.Writer) (n int64, err error) {
	_n, err := w.Write(f.Data.Data)
	return int64(_n), err
}

func (f *TestFrame) Size() uint {
	return f.Data.Len()
}

func (f *TestFrame) StartTime() time.Time {
	return f.Start
}

func (f *TestFrame) EndTime() time.Time {
	return f.End
}

func (f *TestFrame) KeyFrame() bool {
	return false
}

func (f *TestFrame) MediaType() uint32 {
	return 96
}

func (f *TestFrame) StorageType() uint32 {
	return 96
}

func parseTime(value string) time.Time {
	return assert.Must(time.Parse("2006-01-02 15:04:05", value)).In(time.Local)
}

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

var logger = assert.Must(log.Config{
	Level: log.NewAtomicLevelAt(log.DebugLevel),
	Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
		DisableCaller:     true,
		DisableFunction:   true,
		DisableStacktrace: true,
		EncodeLevel:       log.CapitalLevelEncoder,
		EncodeTime:        log.TimeEncoderOfLayout(DefaultTimeLayout),
		EncodeDuration:    log.SecondsDurationEncoder,
	}),
	//OutputPaths: []string{"test.log"},
}.Build())

func main() {
	mysqlConfig := &config.Mysql{
		Username: "root",
		Password: "css66018",
		Host:     "127.0.0.1",
		Port:     3306,
		DBName:   "cvdsrec",
	}
	ch := filePkg.NewChannel("test",
		filePkg.WithDirectory("C:\\Users\\suy\\Documents\\Language\\Go\\cvds-cmu\\data"),
		filePkg.WithCover(time.Minute),
		filePkg.WithFileSize(unit.MeBiByte*10),
		filePkg.WithCheckDeleteInterval(time.Millisecond*300),
		storage.WithChannelLogger(logger),
		filePkg.WithDBMetaManager(
			filePkg.WithDBMetaManagerDSN(mysqlConfig.DSN()),
		),
	)
	buf := make([]byte, 65536)
	ch.Start()
	now := parseTime("2022-12-08 22:00:00")
	for i := 0; i < 1000; i++ {
		ch.Write(&TestFrame{
			Data:  pool.NewData(buf),
			Start: now,
			End:   now.Add(time.Millisecond * 30),
		})
		time.Sleep(time.Millisecond * 30)
		now = now.Add(time.Millisecond * 50)
	}
	<-ch.ClosedWaiter()
}
