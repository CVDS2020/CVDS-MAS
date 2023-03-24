package main

import (
	"fmt"
	"gitee.com/sy_183/common/assert"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	syssvc "gitee.com/sy_183/common/system/service"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/common/uns"
	dbPkg "gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/storage"
	fileStorage "gitee.com/sy_183/cvds-mas/storage/file"
	"gitee.com/sy_183/cvds-mas/storage/meta"
	"github.com/spf13/cobra"
	"os"
	"path"
	"time"
)

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

func init() {
	cobra.MousetrapHelpText = ""
}

type Size struct {
	unit.Size
}

func (s *Size) String() string {
	return s.Size.String()
}

func (s *Size) Set(ss string) error {
	return s.UnmarshalText(uns.StringToBytes(ss))
}

func (s *Size) Type() string {
	return "size"
}

type Level struct {
	log.Level
}

func (l *Level) String() string {
	return l.Level.String()
}

func (l *Level) Set(s string) error {
	if s == "" {
		return nil
	}
	return l.UnmarshalText(uns.StringToBytes(s))
}

func (l *Level) Type() string {
	return "level"
}

type Config struct {
	Help                bool
	Channels            int
	ChannelName         string
	StartInterval       time.Duration
	Cover               time.Duration
	Directory           string
	FileSize            Size
	FileDuration        time.Duration
	CheckDeleteInterval time.Duration
	WriteSize           Size
	WriteInterval       time.Duration
	DisableSync         bool
	WriteBufferSize     Size
	WriteBufferCount    uint
	MaxSyncReqs         int
	MaxDeletingFiles    int
	Meta                struct {
		DB struct {
			DSN                       string
			IgnoreRecordNotFoundError bool
			SlowThreshold             time.Duration
			SkipDefaultTransaction    bool
			PrepareStmt               bool
			MaxOpenConn               int
			MaxIdleConn               int
			LogLevel                  Level
		}
		EarliestIndexesCacheSize    int
		NeedCreatedIndexesCacheSize int
		MaxFiles                    int
		LogLevel                    Level
	}
	LogDirectory string
	LogLevel     Level
}

type Writer struct {
	lifecycle.Lifecycle

	channel *fileStorage.Channel
	delay   time.Duration

	writeSize     unit.Size
	writeInterval time.Duration
	sync          bool
}

func NewWriter(channel *fileStorage.Channel, delay time.Duration, writeSize unit.Size, writeInterval time.Duration, sync bool) *Writer {
	if writeSize == 0 {
		writeSize = unit.MeBiByte
	}
	if writeInterval == 0 {
		writeInterval = time.Second
	}
	if writeInterval < 10*time.Millisecond {
		writeInterval = 10 * time.Millisecond
	}
	w := &Writer{
		channel:       channel,
		delay:         delay,
		writeSize:     writeSize,
		writeInterval: writeInterval,
		sync:          sync,
	}
	w.Lifecycle = lifecycle.NewWithInterruptedRun(w.start, w.run)
	return w
}

func (w *Writer) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	if err := func() error {
		delayer := time.NewTimer(w.delay)
		defer delayer.Stop()
		select {
		case <-delayer.C:
			return nil
		case <-interrupter:
			return lifecycle.NewInterruptedError("", "启动")
		}
	}(); err != nil {
		return err
	}
	channelWaiter := w.channel.StartedWaiter()
	w.channel.Background()
	for {
		select {
		case err := <-channelWaiter:
			return err
		case <-interrupter:
			w.channel.Close(nil)
		}
	}
}

func (w *Writer) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	block := fileStorage.Writer(make([]byte, w.writeSize))
	ticker := time.NewTicker(w.writeInterval)
	defer func() {
		ticker.Stop()
	}()
	last := time.Now()
	for {
		select {
		case now := <-ticker.C:
			if err := w.channel.Write(block, uint(w.writeSize), last, now, storage.StorageTypeBinary.ID, w.sync); err != nil {
				w.channel.Logger().ErrorWith("存储通道写入失败", err)
			}
			last = now
		case <-interrupter:
			w.channel.Shutdown()
			return nil
		}
	}
}

func main() {
	c := new(Config)
	c.FileSize.Size = fileStorage.DefaultFileSize
	c.WriteSize.Size = unit.MeBiByte
	c.WriteBufferSize.Size = fileStorage.DefaultWriteBufferSize
	command := cobra.Command{
		Use:   "file-writer",
		Short: "文件写入程序",
		Long:  "文件写入程序",
		Run:   func(cmd *cobra.Command, args []string) {},
	}
	logger := assert.Must(log.Config{
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
	command.Flags().BoolVarP(&c.Help, "help", "h", false, "显示帮助信息")
	command.Flags().IntVarP(&c.Channels, "channels", "c", 1, "指定通道总数")
	command.Flags().StringVarP(&c.ChannelName, "channel-name", "n", "channel", "指定通道名称前缀")
	command.Flags().DurationVar(&c.StartInterval, "start-interval", 100*time.Millisecond, "指定通道启动间隔")
	command.Flags().DurationVar(&c.Cover, "cover", fileStorage.DefaultCover, "指定覆盖周期")
	command.Flags().StringVarP(&c.Directory, "directory", "d", "", "指定文件保存路径")
	command.Flags().Var(&c.FileSize, "file-size", "指定单个文件大小")
	command.Flags().DurationVar(&c.FileDuration, "file-duration", fileStorage.DefaultFileDuration, "指定文件持续时间")
	command.Flags().DurationVar(&c.CheckDeleteInterval, "check-delete-interval", fileStorage.DefaultCheckDeleteInterval, "指定检查索引是否可以删除的间隔时间")
	command.Flags().VarP(&c.WriteSize, "write-size", "s", "指定每次写入数据大小")
	command.Flags().DurationVarP(&c.WriteInterval, "write-interval", "i", time.Second, "指定写入数据间隔")
	command.Flags().BoolVar(&c.DisableSync, "disable-sync", false, "禁用每次写入数据完成后同步到磁盘")
	command.Flags().VarP(&c.WriteBufferSize, "write-buffer-size", "b", "指定文件写缓冲区大小")
	command.Flags().UintVar(&c.WriteBufferCount, "write-buffer-count", fileStorage.DefaultWriteBufferCount, "指定文件写缓冲区数量")
	command.Flags().IntVar(&c.MaxSyncReqs, "max-sync-reqs", fileStorage.DefaultMaxSyncReqs, "指定最大同步请求数量")
	command.Flags().IntVar(&c.MaxDeletingFiles, "max-deleting-files", fileStorage.DefaultMaxDeletingFiles, "指定最大正在删除中的文件数量")

	command.Flags().StringVarP(&c.Meta.DB.DSN, "meta.db.dsn", "m", "root:css66018@(localhost:3306)/cvdsrec", "指定元数据数据库连接地址")
	command.Flags().BoolVar(&c.Meta.DB.IgnoreRecordNotFoundError, "meta.db.ignore-record-not-found-error", true, "指定元数据数据库忽略未找到记录的日志输出")
	command.Flags().DurationVar(&c.Meta.DB.SlowThreshold, "meta.db.slow-threshold", time.Second, "指定元数据数据库慢SQL记录阈值")
	command.Flags().BoolVar(&c.Meta.DB.SkipDefaultTransaction, "meta.db.skip-default-transaction", true, "指定元数据数据库禁用默认的事务")
	command.Flags().BoolVar(&c.Meta.DB.PrepareStmt, "meta.db.prepare-stmt", false, "指定元数据数据库开启SQL缓存")
	command.Flags().IntVar(&c.Meta.DB.MaxOpenConn, "meta.db.max-open-conn", 3, "指定元数据数据库最大连接数")
	command.Flags().IntVar(&c.Meta.DB.MaxIdleConn, "meta.db.max-idle-conn", 3, "指定元数据数据库最大活动连接数")
	command.Flags().Var(&c.Meta.DB.LogLevel, "meta.db.log-level", "指定元数据数据库管理器日志级别")

	command.Flags().IntVar(&c.Meta.EarliestIndexesCacheSize, "meta.earliest-indexes-cache-size", meta.DefaultEarliestIndexesCacheSize, "指定最早的索引缓存数量")
	command.Flags().IntVar(&c.Meta.NeedCreatedIndexesCacheSize, "meta.need-created-indexes-cache-size", meta.DefaultNeedCreatedIndexesCacheSize, "指定未刷新到磁盘中的索引缓存数量")
	command.Flags().IntVar(&c.Meta.MaxFiles, "meta.max-files", meta.DefaultMaxFiles, "指定最大文件数量")
	command.Flags().Var(&c.Meta.LogLevel, "meta.log-level", "指定元数据管理器日志等级")
	command.Flags().StringVar(&c.LogDirectory, "log-directory", "", "指定日志输出目录未知，为空则输出到标准输出")
	command.Flags().Var(&c.LogLevel, "log-level", "指定存储通道日志等级")
	err := command.Execute()
	if err != nil {
		logger.Fatal("解析命令行参数失败", log.Error(err))
	}
	if c.Help {
		return
	}
	if c.Channels == 0 {
		logger.Warn("没有指定任何通道，无法启动存储")
		return
	}

	channels := make([]*fileStorage.Channel, 0, c.Channels)
	for i := 0; i < c.Channels; i++ {
		name := fmt.Sprintf("%s_%d", c.ChannelName, i)
		dbManager := dbPkg.NewDBManager(nil, c.Meta.DB.DSN,
			dbPkg.WithTablesInfo(meta.MetaTablesInfos),
			dbPkg.WithIgnoreRecordNotFoundError(c.Meta.DB.IgnoreRecordNotFoundError),
			dbPkg.WithSlowThreshold(c.Meta.DB.SlowThreshold),
			dbPkg.WithSkipDefaultTransaction(c.Meta.DB.SkipDefaultTransaction),
			dbPkg.WithPrepareStmt(c.Meta.DB.PrepareStmt),
			dbPkg.WithMaxOpenConn(c.Meta.DB.MaxOpenConn),
			dbPkg.WithMaxIdleConn(c.Meta.DB.MaxIdleConn),
		)
		channel := fileStorage.NewChannel(name,
			storage.WithCover(c.Cover),
			fileStorage.WithDirectory(path.Join(c.Directory, name)),
			fileStorage.WithFileSize(c.FileSize.Size),
			fileStorage.WithFileDuration(c.FileDuration),
			fileStorage.WithCheckDeleteInterval(c.CheckDeleteInterval),
			fileStorage.WithWriteBufferPoolConfig(c.WriteBufferSize.Uint(), pool.StackPoolProvider[*fileStorage.Buffer](c.WriteBufferCount)),
			fileStorage.WithMaxSyncReqs(c.MaxSyncReqs),
			fileStorage.WithMaxDeletingFiles(c.MaxDeletingFiles),
			fileStorage.WithMetaManagerConfig(meta.ProvideDBMetaManager,
				meta.WithDBManager(dbManager),
				meta.WithEarliestIndexesCacheSize(c.Meta.EarliestIndexesCacheSize),
				meta.WithNeedCreatedIndexesCacheSize(c.Meta.NeedCreatedIndexesCacheSize),
				meta.WithMaxFiles(c.Meta.MaxFiles),
			),
		)
		logCfg := log.Config{
			Level: log.NewAtomicLevelAt(c.LogLevel.Level),
			Encoder: log.NewConsoleEncoder(log.ConsoleEncoderConfig{
				DisableCaller:     true,
				DisableFunction:   true,
				DisableStacktrace: true,
				EncodeLevel:       log.CapitalColorLevelEncoder,
				EncodeTime:        log.TimeEncoderOfLayout(DefaultTimeLayout),
				EncodeDuration:    log.SecondsDurationEncoder,
			}),
		}
		if c.LogDirectory != "" {
			logCfg.OutputPaths = []string{path.Join(c.LogDirectory, name+".log")}
		}
		chLogger, err := logCfg.Build()
		if err != nil {
			logger.Fatal("创建日志失败", log.Error(err))
		}
		channel.SetLogger(chLogger.Named(channel.DisplayName()))

		metaLogger := chLogger
		if c.Meta.LogLevel.Level > c.LogLevel.Level {
			metaLogger = chLogger.WithOptions(log.IncreaseLevel(c.Meta.LogLevel.Level))
		}
		metaManager := channel.MetaManager()
		metaManager.SetLogger(metaLogger.Named(metaManager.DisplayName()))

		metaDBLogger := chLogger
		if c.Meta.DB.LogLevel.Level > c.LogLevel.Level {
			metaDBLogger = chLogger.WithOptions(log.IncreaseLevel(c.Meta.DB.LogLevel.Level))
		}
		dbManager.SetLogger(metaDBLogger)

		channel.OnStarting(func(lifecycle.Lifecycle) {
			channel.Logger().Info("存储通道正在启动")
		}).OnStarted(func(l lifecycle.Lifecycle, err error) {
			if err != nil {
				channel.Logger().ErrorWith("存储通道启动失败", err)
			} else {
				channel.Logger().Info("存储通道启动完成")
			}
		}).OnClose(func(l lifecycle.Lifecycle, err error) {
			channel.Logger().Info("存储通道正在关闭")
		}).OnClosed(func(l lifecycle.Lifecycle, err error) {
			channel.Logger().Info("存储通道已关闭")
		})

		channels = append(channels, channel)
	}

	group := lifecycle.NewGroup()
	var delay time.Duration
	for _, channel := range channels {
		group.Add(channel.Name(), NewWriter(channel, delay, c.WriteSize.Size, c.WriteInterval, !c.DisableSync))
		delay += c.StartInterval
	}
	os.Exit(syssvc.New(command.Use, group, syssvc.ErrorCallback(syssvc.LogErrorCallback(logger))).Run())
}
