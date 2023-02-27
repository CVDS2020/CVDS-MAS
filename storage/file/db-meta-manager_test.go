package file

import (
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/storage"
	"testing"
	"time"
)

func TestDBMetaManagerV2(t *testing.T) {
	ch := NewChannel("test")
	mysqlConfig := &config.Mysql{
		Username: "root",
		Password: "css66018",
		Host:     "127.0.0.1",
		Port:     3306,
		DBName:   "cvdsrec",
	}
	m := NewDBMetaManager(ch,
		WithDBMetaManagerDSN(mysqlConfig.DSN()),
		WithDBMetaManagerLogger(Logger()),
	)
	m.Start()
	for i := 0; i < 100; i++ {
		now := time.Now()
		file := m.NewFile(now.UnixMilli(), media.MediaTypePS.ID, storage.StorageTypeRTP.ID)
		m.AddFile(file)
		var offset uint64
		for j := 0; j < 100; j++ {
			now = time.Now()
			m.BuildIndex(&Index{
				Start:      now.UnixMilli(),
				End:        now.Add(time.Millisecond * 1000).UnixMilli(),
				FileSeq:    file.Seq,
				FileOffset: offset,
				Size:       unit.MeBiByte,
			})
			offset += unit.MeBiByte
			time.Sleep(time.Millisecond * 1200)
		}
	}
	m.Shutdown()
}
