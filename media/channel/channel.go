package channel

import (
	"fmt"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/common/pool"
	"gitee.com/sy_183/common/unit"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/media"
	"gitee.com/sy_183/cvds-mas/storage"
	fileStorage "gitee.com/sy_183/cvds-mas/storage/file"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const ChannelModule = Module + ".channel"

const (
	DefaultStorageBufferSize = unit.MeBiByte
	MinStorageBufferSize     = 64 * unit.KiBiByte
)

type Channel struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	id                string
	cover             atomic.Int64
	storageType       string
	storageBufferSize uint

	rtpPlayer     atomic.Pointer[RtpPlayer]
	rtpPlayer2    atomic.Pointer[RtpPlayer2]
	rtpPlayerLock sync.RWMutex

	storageChannel     atomic.Pointer[storage.Channel]
	storageChannelLock sync.Mutex
	enableRecord       atomic.Bool

	rtpPushers     container.SyncMap[uint64, *RtpPusher]
	rtpPushers2    container.SyncMap[uint64, *RtpPusher2]
	rtpPushersId   uint64
	rtpPushersLock sync.Mutex

	historyRtpPushers     container.SyncMap[uint64, *HistoryRtpPusher]
	historyRtpPusherId    uint64
	historyRtpPushersLock sync.Mutex

	log.AtomicLogger
}

func NewChannel(name string, options ...Option) *Channel {
	c := &Channel{
		id: name,
	}

	for _, option := range options {
		option.Apply(c)
	}

	if c.storageBufferSize == 0 {
		c.storageBufferSize = DefaultStorageBufferSize
	}
	if c.storageBufferSize < MinStorageBufferSize {
		c.storageBufferSize = MinStorageBufferSize
	}
	if !c.CompareAndSwapLogger(nil, config.DefaultLogger().Named(c.DisplayName())) {
		c.SetLogger(c.Logger().Named(c.DisplayName()))
	}
	c.runner = lifecycle.NewWithInterruptedRun(nil, c.run, lifecycle.WithSelf(c))
	c.Lifecycle = c.runner
	return c
}

func (c *Channel) DisplayName() string {
	return fmt.Sprintf("媒体通道(%s)", c.id)
}

func (c *Channel) RtpPlayerDisplayName() string {
	return fmt.Sprintf("媒体通道(%s)RTP拉流服务", c.id)
}

func (c *Channel) RtpPusherDisplayName(id uint64) string {
	return fmt.Sprintf("媒体通道(%s)实时音视频RTP推流服务(%d)", c.id, id)
}

func (c *Channel) HistoryRtpPlayerDisplayName(id uint64) string {
	return fmt.Sprintf("媒体通道(%s)历史音视频RTP推流服务(%d)", c.id, id)
}

func (c *Channel) ID() string {
	return c.id
}

func (c *Channel) Cover() time.Duration {
	return time.Duration(c.cover.Load())
}

func (c *Channel) StorageType() string {
	return c.storageType
}

func (c *Channel) StorageBufferSize() uint {
	return c.storageBufferSize
}

func (c *Channel) SetField(name string, value any) lifecycle.Lifecycle {
	l := c.Lifecycle.SetField(name, value)
	if storageChannel := c.StorageChannel(); storageChannel != nil {
		storageChannel.SetField(name, value)
	}
	return l
}

func (c *Channel) SetDefaultField(name string, defaultValue any) (value any, exist bool) {
	if value, exist = c.Lifecycle.SetDefaultField(name, defaultValue); exist {
		return
	}
	if storageChannel := c.StorageChannel(); storageChannel != nil {
		storageChannel.SetDefaultField(name, defaultValue)
	}
	return
}

func (c *Channel) DeleteField(name string) lifecycle.Lifecycle {
	if storageChannel := c.StorageChannel(); storageChannel != nil {
		storageChannel.DeleteField(name)
	}
	return c.Lifecycle.DeleteField(name)
}

func (c *Channel) RemoveField(name string) any {
	if storageChannel := c.StorageChannel(); storageChannel != nil {
		storageChannel.RemoveField(name)
	}
	return c.Lifecycle.RemoveField(name)
}

func (c *Channel) Info() map[string]any {
	return map[string]any{
		"id":           c.ID(),
		"storageCover": c.Cover(),
		"fields":       c.Fields(),
	}
}

func (c *Channel) StorageChannel() storage.Channel {
	if scp := c.storageChannel.Load(); scp != nil {
		return *scp
	}
	return nil
}

func (c *Channel) loadEnabledStorageChannel() storage.Channel {
	if c.enableRecord.Load() {
		return c.StorageChannel()
	}
	return nil
}

func (c *Channel) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	<-interrupter
	var historyRTPPusherFutures []<-chan error
	c.historyRtpPushers.Range(func(id uint64, pusher *HistoryRtpPusher) bool {
		pusher.Close(nil)
		historyRTPPusherFutures = append(historyRTPPusherFutures, pusher.ClosedWaiter())
		return true
	})
	for _, future := range historyRTPPusherFutures {
		<-future
	}
	if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
		rtpPlayer.Shutdown()
	}
	if rtpPlayer := c.rtpPlayer2.Load(); rtpPlayer != nil {
		rtpPlayer.Shutdown()
	}
	if storageChannel := c.StorageChannel(); storageChannel != nil {
		storageChannel.Shutdown()
	}
	return nil
}

func (c *Channel) OpenRtpPlayer2(closedCallback RtpPlayer2CloseCallback) (*RtpPlayer2, error) {
	return lock.RLockGetDouble(c.runner, func() (*RtpPlayer2, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开RTP拉流失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		return lock.LockGetDouble(&c.rtpPlayerLock, func() (*RtpPlayer2, error) {
			if rtpPlayer := c.rtpPlayer2.Load(); rtpPlayer != nil {
				return rtpPlayer, errors.NewExist(c.RtpPlayerDisplayName())
			}
			player := NewRtpPlayer2(c)
			player.OnClosed(func(l lifecycle.Lifecycle, err error) {
				if closedCallback != nil {
					closedCallback(player, c)
				}
				player.Logger().Info("RTP拉流服务已关闭")
				lock.LockDo(&c.rtpPlayerLock, func() { c.rtpPlayer2.Store(nil) })
			}).OnClose(func(l lifecycle.Lifecycle, err error) {
				player.Logger().Info("RTP拉流服务执行关闭操作成功")
			}).Background()

			c.rtpPlayer2.Store(player)
			return player, nil
		})
	})
}

func (c *Channel) GetRtpPlayer2() *RtpPlayer2 {
	return c.rtpPlayer2.Load()
}

func (c *Channel) RtpPlayerAddStream(transport string) (streamId uint64, err error) {
	if rtpPlayer := c.rtpPlayer2.Load(); rtpPlayer != nil {
		return rtpPlayer.AddStream(transport)
	}
	return 0, c.Logger().ErrorWith("RTP拉流添加失败", errors.NewNotFound(c.RtpPlayerDisplayName()))
}

func (c *Channel) RtpPlayerSetupStream(streamId uint64, remoteIP net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo, config SetupConfig) error {
	if rtpPlayer := c.rtpPlayer2.Load(); rtpPlayer != nil {
		return rtpPlayer.SetupStream(streamId, remoteIP, remotePort, ssrc, streamInfo, config)
	}
	return c.Logger().ErrorWith("RTP拉流设置失败", errors.NewNotFound(c.RtpPlayerDisplayName()))
}

func (c *Channel) RtpPlayerRemoveStream(streamId uint64) error {
	if rtpPlayer := c.rtpPlayer2.Load(); rtpPlayer != nil {
		return rtpPlayer.RemoveStream(streamId)
	}
	return c.Logger().ErrorWith("RTP拉流移除失败", errors.NewNotFound(c.RtpPlayerDisplayName()))
}

func (c *Channel) CloseRtpPlayer2() <-chan error {
	if rtpPlayer := c.rtpPlayer2.Load(); rtpPlayer != nil {
		rtpPlayer.Close(nil)
		return rtpPlayer.ClosedWaiter()
	}
	return nil
}

func (c *Channel) OpenRtpPlayer(transport string, timeout time.Duration, closedCallback RtpPlayerCloseCallback) (*RtpPlayer, error) {
	return lock.RLockGetDouble(c.runner, func() (*RtpPlayer, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开RTP拉流失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		return lock.LockGetDouble(&c.rtpPlayerLock, func() (*RtpPlayer, error) {
			if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
				return rtpPlayer, errors.NewExist(c.RtpPlayerDisplayName())
			}
			player, err := NewRTPPlayer(c, transport, timeout)
			if err != nil {
				return nil, err
			}
			player.OnClosed(func(l lifecycle.Lifecycle, err error) {
				if closedCallback != nil {
					closedCallback(player, c)
				}
				player.Logger().Info("RTP拉流服务已关闭")
				lock.LockDo(&c.rtpPlayerLock, func() { c.rtpPlayer.Store(nil) })
			}).OnClose(func(l lifecycle.Lifecycle, err error) {
				player.Logger().Info("RTP拉流服务执行关闭操作成功")
			}).Background()
			c.Logger().Info("创建RTP拉流服务成功")

			c.rtpPlayer.Store(player)
			return player, nil
		})
	})
}

// SetupRtpPlayer 设置通道RTP拉流参数
// error@StateNotRunningError 通道未运行
// error@NotFound RTP拉流服务不存在
func (c *Channel) SetupRtpPlayer(rtpMap map[uint8]string, remoteIP net.IP, remotePort int, ssrc int64, once bool) error {
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return c.Logger().ErrorWith("设置RTP拉流参数失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
			return rtpPlayer.Setup(rtpMap, remoteIP, remotePort, ssrc, once)
		}
		return c.Logger().ErrorWith("设置RTP拉流参数成功", errors.NewNotFound(c.RtpPlayerDisplayName()))
	})
}

func (c *Channel) GetRtpPlayer() *RtpPlayer {
	return c.rtpPlayer.Load()
}

func (c *Channel) CloseRtpPlayer() <-chan error {
	if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
		rtpPlayer.Close(nil)
		return rtpPlayer.ClosedWaiter()
	}
	return nil
}

func (c *Channel) SetupStorage(cover time.Duration) error {
	if cover < time.Minute {
		cover = time.Minute
	}
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return c.Logger().ErrorWith("设置存储配置失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}

		return lock.LockGet(&c.storageChannelLock, func() error {
			if storageChannel := c.StorageChannel(); storageChannel == nil {
				// 存储通道还未创建，创建并启动存储通道
				s := fileStorage.GetStorage()
				storageChannel, _ = s.NewChannel(c.id, cover,
					fileStorage.WithWriteBufferPoolConfig(c.storageBufferSize, pool.StackPoolProvider[*fileStorage.Buffer](2)))
				c.cover.Store(int64(cover))
				for name, value := range c.Fields() {
					storageChannel.SetField(name, value)
				}
				storageChannel.OnClosed(func(l lifecycle.Lifecycle, err error) {
					// 已关闭的存储通道将其删除
					storageChannel.Logger().Info("存储通道已关闭")
					lock.LockDo(&c.storageChannelLock, func() { c.storageChannel.Store(nil) })
				}).OnStarted(func(l lifecycle.Lifecycle, err error) {
					if err != nil {
						// 启动失败的存储通道将其删除
						lock.LockDo(&c.storageChannelLock, func() { c.storageChannel.Store(nil) })
					} else {
						storageChannel.Logger().Info("存储通道启动成功")
					}
				}).OnClose(func(l lifecycle.Lifecycle, err error) {
					storageChannel.Logger().Info("存储通道执行关闭操作成功")
				}).Background()
				c.storageChannel.Store(&storageChannel)
				c.Logger().Info("存储通道创建成功", log.Duration("覆盖周期", cover))
			} else {
				// 更改存储覆盖周期
				old := storageChannel.Cover()
				storageChannel.SetCover(cover)
				if old != cover {
					c.Logger().Info("存储通道覆盖周期修改成功", log.Duration("覆盖周期", cover))
				}
			}
			return nil
		})
	})
}

func (c *Channel) StopStorage() <-chan error {
	if storageChannel := c.StorageChannel(); storageChannel != nil {
		storageChannel.Close(nil)
		return storageChannel.ClosedWaiter()
	}
	return nil
}

func (c *Channel) StartRecord() error {
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return c.Logger().ErrorWith("启动录像失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		if c.enableRecord.CompareAndSwap(false, true) {
			c.Logger().Info("启动录像成功")
		}
		return nil
	})
}

func (c *Channel) StopRecord() {
	if c.enableRecord.CompareAndSwap(true, false) {
		if storageChannel := c.StorageChannel(); storageChannel != nil {
			storageChannel.Sync()
		}
		c.Logger().Info("停止录像成功")
	}
	return
}

func (c *Channel) allocRtpPusherId() uint64 {
	id := c.rtpPushersId
	c.rtpPushersId++
	return id
}

func (c *Channel) OpenRtpPusher2(maxCached int, closedCallback RtpPusher2ClosedCallback) (*RtpPusher2, error) {
	return lock.RLockGetDouble(c.runner, func() (*RtpPusher2, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开实时音视频RTP推流失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		id := c.allocRtpPusherId()
		pusher := NewRtpPusher2(id, c, maxCached)
		pusher.OnClosed(func(l lifecycle.Lifecycle, err error) {
			if closedCallback != nil {
				closedCallback(pusher, c)
			}
			pusher.Logger().Info("实时音视频RTP推流服务已关闭")
			// 已关闭的实时音视频RTP推流将其从通道中删除
			c.rtpPushers.Delete(id)
		}).OnClose(func(l lifecycle.Lifecycle, err error) {
			pusher.Logger().Info("实时音视频RTP推流服务执行关闭操作成功")
		}).Start()
		c.rtpPushers2.Store(id, pusher)
		c.Logger().Info("实时音视频RTP推流服务创建成功", log.Uint64("id", id))
		return pusher, nil
	})
}

func (c *Channel) GetRtpPusher2(id uint64) *RtpPusher2 {
	rtpPusher, _ := c.rtpPushers2.Load(id)
	return rtpPusher
}

func (c *Channel) RtpPusher2s() []*RtpPusher2 {
	return c.rtpPushers2.Values()
}

func (c *Channel) RtpPusherAddStream(id uint64, transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (streamId uint64, err error) {
	if rtpPusher, _ := c.rtpPushers2.Load(id); rtpPusher != nil {
		return rtpPusher.AddStream(transport, remoteIp, remotePort, ssrc, streamInfo)
	}
	return 0, c.Logger().ErrorWith("RTP推流添加失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) RtpPusherRemoveStream(id uint64, streamId uint64) error {
	if rtpPusher, _ := c.rtpPushers2.Load(id); rtpPusher != nil {
		return rtpPusher.RemoveStream(streamId)
	}
	return c.Logger().ErrorWith("RTP推流移除失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) CloseRtpPusher2(id uint64) <-chan error {
	if rtpPusher, _ := c.rtpPushers2.Load(id); rtpPusher != nil {
		rtpPusher.Close(nil)
		return rtpPusher.ClosedWaiter()
	}
	return nil
}

func (c *Channel) OpenRtpPusher(targetIp net.IP, targetPort int, transport string, ssrc int64, closedCallback RtpPusherClosedCallback) (*RtpPusher, error) {
	return lock.RLockGetDouble(c.runner, func() (*RtpPusher, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开实时音视频RTP推流失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		id := c.allocRtpPusherId()
		pusher, err := NewRtpPusher(id, c, targetIp, targetPort, transport, ssrc)
		if err != nil {
			return nil, c.Logger().ErrorWith("打开实时音视频RTP推流失败", err)
		}
		pusher.OnClosed(func(l lifecycle.Lifecycle, err error) {
			if closedCallback != nil {
				closedCallback(pusher, c)
			}
			pusher.Logger().Info("实时音视频RTP推流服务已关闭")
			// 已关闭的实时音视频RTP推流将其从通道中删除
			c.rtpPushers.Delete(id)
		}).OnStarted(func(l lifecycle.Lifecycle, err error) {
			if err != nil {
				if closedCallback != nil {
					closedCallback(pusher, c)
				}
				// 启动失败的实时音视频RTP推流将其从通道中删除
				c.rtpPushers.Delete(id)
			} else {
				pusher.Logger().Info("实时音视频RTP推流服务启动成功")
			}
		}).OnClose(func(l lifecycle.Lifecycle, err error) {
			pusher.Logger().Info("实时音视频RTP推流服务执行关闭操作成功")
		}).Start()
		c.rtpPushers.Store(id, pusher)
		c.Logger().Info("实时音视频RTP推流服务创建成功", log.Uint64("id", id))
		return pusher, nil
	})
}

func (c *Channel) RtpPushers() []*RtpPusher {
	return c.rtpPushers.Values()
}

func (c *Channel) StartRtpPusher(id uint64) error {
	if pusher, _ := c.rtpPushers.Load(id); pusher != nil {
		pusher.StartPush()
		return nil
	}
	return c.Logger().ErrorWith("设置实时音视频RTP推流服务开始推流失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) CloseRtpPusher(id uint64) <-chan error {
	if pusher, _ := c.rtpPushers.Load(id); pusher != nil {
		pusher.Close(nil)
		return pusher.ClosedWaiter()
	}
	return nil
}

func (c *Channel) allocHistoryRtpPusherID() uint64 {
	id := c.historyRtpPusherId
	c.historyRtpPusherId++
	return id
}

func (c *Channel) OpenHistoryRtpPusher(targetIp net.IP, targetPort int, transport string, startTime, endTime int64, ssrc int64,
	eofCallback HistoryRtpPusherEOFCallback, closedCallback HistoryRtpPusherClosedCallback) (*HistoryRtpPusher, error) {
	return lock.RLockGetDouble(c.runner, func() (*HistoryRtpPusher, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开历史音视频RTP推流失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		id := c.allocHistoryRtpPusherID()
		pusher, err := NewHistoryRtpPusher(id, c, targetIp, targetPort, transport, startTime, endTime, ssrc, eofCallback)
		if err != nil {
			return nil, c.Logger().ErrorWith("打开历史音视频RTP推流失败", err)
		}
		pusher.OnClosed(func(l lifecycle.Lifecycle, err error) {
			if closedCallback != nil {
				closedCallback(pusher, c)
			}
			pusher.Logger().Info("历史音视频RTP推流服务已关闭")
			// 已关闭的历史音视频RTP推流将其从通道中删除
			c.historyRtpPushers.Delete(id)
		}).OnStarted(func(l lifecycle.Lifecycle, err error) {
			if err != nil {
				if closedCallback != nil {
					closedCallback(pusher, c)
				}
				// 启动失败的历史音视频RTP推流将其从通道中删除
				c.historyRtpPushers.Delete(id)
			} else {
				pusher.Logger().Info("历史音视频RTP推流服务启动成功")
			}
		}).OnClose(func(l lifecycle.Lifecycle, err error) {
			pusher.Logger().Info("历史音视频RTP推流服务执行关闭操作成功")
		}).Background()
		c.historyRtpPushers.Store(id, pusher)
		c.Logger().Info("历史音视频RTP推流服务创建成功", log.Uint64("id", id))
		return pusher, nil
	})
}

func (c *Channel) HistoryRtpPushers() []*HistoryRtpPusher {
	return c.historyRtpPushers.Values()
}

func (c *Channel) StartHistoryRtpPusher(id uint64) error {
	if pusher, _ := c.historyRtpPushers.Load(id); pusher != nil {
		pusher.StartPush()
		return nil
	}
	return c.Logger().ErrorWith("设置历史音视频RTP推流服务开始推流失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}

func (c *Channel) GetHistoryRtpPusher(id uint64) *HistoryRtpPusher {
	if pusher, _ := c.historyRtpPushers.Load(id); pusher != nil {
		return pusher
	}
	return nil
}

func (c *Channel) PauseHistoryRtpPusher(id uint64) error {
	if pusher, _ := c.historyRtpPushers.Load(id); pusher != nil {
		pusher.Logger().Debug("历史音视频RTP推流服务请求暂停")
		pusher.Pause()
		return nil
	}
	return c.Logger().ErrorWith("暂停历史音视频RTP推流失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}

func (c *Channel) ResumeHistoryRtpPusher(id uint64) error {
	if pusher, _ := c.historyRtpPushers.Load(id); pusher != nil {
		pusher.Logger().Debug("历史音视频RTP推流服务请求恢复")
		pusher.Resume()
		return nil
	}
	return c.Logger().ErrorWith("恢复历史音视频RTP推流失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}

func (c *Channel) SeekHistoryRtpPusher(id uint64, t int64) error {
	if pusher, _ := c.historyRtpPushers.Load(id); pusher != nil {
		pusher.Logger().Debug("历史音视频RTP推流服务请求定位", log.Time("定位时间", time.UnixMilli(t)))
		return pusher.Seek(t)
	}
	return c.Logger().ErrorWith("定位历史音视频RTP推流失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}

func (c *Channel) SetHistoryRtpPusherScale(id uint64, scale float64) error {
	if pusher, _ := c.historyRtpPushers.Load(id); pusher != nil {
		pusher.Logger().Debug("历史音视频RTP推流服务请求设置倍速", log.Float64("倍速", scale))
		return pusher.SetScale(scale)
	}
	return c.Logger().ErrorWith("设置历史音视频RTP推流倍速失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}

func (c *Channel) CloseHistoryRtpPusher(id uint64) <-chan error {
	if pusher, _ := c.historyRtpPushers.Load(id); pusher != nil {
		pusher.Close(nil)
		return pusher.ClosedWaiter()
	}
	return nil
}
