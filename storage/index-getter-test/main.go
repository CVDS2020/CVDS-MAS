package main

import (
	"bufio"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/log"
	dbPkg "gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/storage"
	fileStorage "gitee.com/sy_183/cvds-mas/storage/file"
	"gitee.com/sy_183/cvds-mas/storage/meta"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

func parseTime(value string) time.Time {
	return assert.Must(time.Parse("2006-01-02 15:04:05", value)).In(time.Local)
}

var logger = assert.Must(log.Config{
	Level: log.NewAtomicLevelAt(log.DebugLevel),
	Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
		DisableCaller:     true,
		DisableFunction:   true,
		DisableStacktrace: true,
		EncodeLevel:       log.CapitalColorLevelEncoder,
		EncodeTime:        log.TimeEncoderOfLayout(DefaultTimeLayout),
		EncodeDuration:    log.SecondsDurationEncoder,
	}),
}.Build())

func main() {
	dbManager := dbPkg.NewDBManager(nil, "root:css66018@(localhost:3306)/cvdsrec")
	ch := fileStorage.NewChannel("44010200491320000003",
		storage.WithCover(90*24*time.Hour),
		fileStorage.WithDirectory("C:\\Users\\suy\\Documents\\Language\\Go\\cvds-cmu\\data\\44010200491320000003"),
		fileStorage.WithMetaManagerConfig(meta.ProvideDBMetaManager, meta.WithDBManager(dbManager)),
		storage.WithLogger(logger),
	)
	ch.Start()
	indexGetter := storage.NewCachedIndexGetter(ch, parseTime("2023-02-10 6:30:00").UnixMilli(), parseTime("2023-02-10 8:00:00").UnixMilli(), 0, 0)
	indexGetter.Start()

	cmdScanner := bufio.NewScanner(os.Stdin)
	for cmdScanner.Scan() {
		cmd := strings.TrimSpace(cmdScanner.Text())
		if cmd == "exit" {
			break
		}
		if cmd == "get" {
			index, err := indexGetter.Get()
			if err != nil {
				if err == io.EOF {
					logger.Warn("索引获取到结尾")
				} else {
					logger.ErrorWith("获取索引失败", err)
				}
				continue
			}
			logger.Info("获取索引成功", log.Object("索引", index))
			continue
		}
		if strings.HasPrefix(cmd, "seek ") {
			ts := strings.TrimSpace(cmd[len("seek "):])
			t, err := strconv.ParseInt(ts, 10, 64)
			if err != nil {
				logger.ErrorWith("解析seek参数失败", err)
				continue
			}
			indexGetter.Seek(t)
			continue
		}
		logger.Warn("未知的命令", log.String("命令", cmd))
	}

	indexGetter.Shutdown()
	ch.Shutdown()
}
