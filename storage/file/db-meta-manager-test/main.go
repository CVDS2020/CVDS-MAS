package main

import (
	"fmt"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/storage"
	filePkg "gitee.com/sy_183/cvds-mas/storage/file"
	"time"
)

func parseTime(value string) time.Time {
	return assert.Must(time.Parse("2006-01-02 15:04:05", value)).In(time.Local)
}

func main() {
	ch := filePkg.NewChannel("test")
	mysqlConfig := &config.Mysql{
		Username: "root",
		Password: "css66018",
		Host:     "127.0.0.1",
		Port:     3306,
		DBName:   "cvdsrec",
	}
	m := filePkg.NewDBMetaManager(ch,
		filePkg.WithDBMetaManagerDSN(mysqlConfig.DSN()),
		filePkg.WithDBMetaManagerLogger(filePkg.Logger()),
	)
	m.Start()
	now := parseTime("2022-12-08 22:00:00")
	for i := 2; i < 20; i += 2 {
		file := m.NewFile(now.UnixMilli(), media.MediaTypePS.ID, storage.StorageTypeRTP.ID)
		file.Seq = uint64(i)
		m.AddFile(file)
		var offset uint64
		for j := 0; j < 1; j++ {
			index := m.BuildIndex(&filePkg.Index{
				Start:      now.UnixMilli(),
				End:        now.Add(time.Millisecond * 30).UnixMilli(),
				FileSeq:    file.Seq,
				FileOffset: offset,
				Size:       unit.MeBiByte,
			})
			m.AddIndex(index)
			now = now.Add(time.Millisecond * 50)
			offset += unit.MeBiByte
			time.Sleep(time.Millisecond * 30)
		}
	}
	files := m.GetFiles(nil, 67, -1, -1)
	for _, file := range files {
		fmt.Println(file)
	}
	indexes := assert.Must(m.FindIndexes(
		parseTime("2022-12-08 22:00:40").UnixMilli(),
		parseTime("2022-12-08 22:00:50").UnixMilli(),
		-1,
		nil,
	))
	for _, index := range indexes {
		fmt.Println(index)
	}
	<-m.ClosedWaiter()
}
