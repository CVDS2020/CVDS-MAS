package channel

import (
	"fmt"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	errPkg "gitee.com/sy_183/cvds-mas/errors"
	"gitee.com/sy_183/cvds-mas/storage"
	"gitee.com/sy_183/cvds-mas/storage/file"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

type Channel struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	id          string
	cover       atomic.Int64
	storageType *storage.StorageType

	rtpPlayer     atomic.Pointer[RTPPlayer]
	rtpPlayerLock sync.RWMutex

	storageChannel     atomic.Pointer[storage.Channel]
	storageChannelLock sync.Mutex
	enableRecord       atomic.Bool

	historyRTPPushers     container.SyncMap[uint64, *HistoryRTPPusher]
	historyRTPPusherID    uint64
	historyRTPPushersLock sync.Mutex

	log.AtomicLogger
}

func NewChannel(name string, options ...Option) *Channel {
	c := &Channel{
		id: name,
	}
	c.storageType = &storage.StorageTypeRTP

	for _, option := range options {
		option.Apply(c)
	}

	c.CompareAndSwapLogger(nil, Logger().Named(c.DisplayName()))
	c.runner = lifecycle.NewWithInterruptedRun(nil, c.run)
	c.Lifecycle = c.runner
	return c
}

func (c *Channel) ID() string {
	return c.id
}

func (c *Channel) DisplayName() string {
	return fmt.Sprintf("媒体通道(%s)", c.id)
}

func (c *Channel) RTPPlayerDisplayName() string {
	return fmt.Sprintf("媒体通道(%s)RTP拉流服务", c.id)
}

func (c *Channel) HistoryRTPPlayerDisplayName(id uint64) string {
	return fmt.Sprintf("媒体通道(%s)历史音视频RTP推流服务(%d)", c.id, id)
}

func (c *Channel) Cover() time.Duration {
	return time.Duration(c.cover.Load())
}

func (c *Channel) ModifyField(name string, value any) bool {
	old, exist := c.SetDefaultField(name, value)
	if exist {
		if reflect.TypeOf(old).Comparable() && reflect.TypeOf(value).Comparable() && old == value {
			return false
		}
	}
	if storageChannel := c.loadStorageChannel(); storageChannel != nil {
		storageChannel.SetField(name, value)
	}
	return true
}

func (c *Channel) StorageType() *storage.StorageType {
	return c.storageType
}

func (c *Channel) Info() map[string]any {
	return map[string]any{
		"id":           c.ID(),
		"storageCover": c.Cover(),
		"storageType":  c.StorageType().Name,
		"fields":       c.Fields(),
	}
}

func (c *Channel) loadStorageChannel() storage.Channel {
	if scp := c.storageChannel.Load(); scp != nil {
		return *scp
	}
	return nil
}

func (c *Channel) loadEnabledStorageChannel() storage.Channel {
	if c.enableRecord.Load() {
		return c.loadStorageChannel()
	}
	return nil
}

func (c *Channel) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	<-interrupter
	var historyRTPPusherFutures []<-chan error
	c.historyRTPPushers.Range(func(id uint64, pusher *HistoryRTPPusher) bool {
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
	if storageChannel := c.loadStorageChannel(); storageChannel != nil {
		storageChannel.Shutdown()
	}
	return nil
}

func (c *Channel) OpenRTPPlayer(transport string, timeout time.Duration, closedCallback RTPPlayerCloseCallback) (*RTPPlayer, error) {
	return lock.RLockGetDouble(c.runner, func() (*RTPPlayer, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开RTP拉流失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		return lock.LockGetDouble(&c.rtpPlayerLock, func() (*RTPPlayer, error) {
			if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
				return rtpPlayer, errPkg.NewExist(c.RTPPlayerDisplayName())
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

// SetupRTPPlayer 设置通道RTP拉流参数
// error@StateNotRunningError 通道未运行
// error@NotFound RTP拉流服务不存在
func (c *Channel) SetupRTPPlayer(rtpMap map[uint8]string, remoteIP net.IP, remotePort int, ssrc int64, once bool) error {
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return c.Logger().ErrorWith("设置RTP拉流参数失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
			return rtpPlayer.Setup(rtpMap, remoteIP, remotePort, ssrc, once)
		}
		return c.Logger().ErrorWith("设置RTP拉流参数成功", errPkg.NewNotFound(c.RTPPlayerDisplayName()))
	})
}

func (c *Channel) GetRTPPlayer() *RTPPlayer {
	return c.rtpPlayer.Load()
}

func (c *Channel) CloseRTPPlayer() <-chan error {
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
			if storageChannel := c.loadStorageChannel(); storageChannel == nil {
				// 存储通道还未创建，创建并启动存储通道
				s := file.GetStorage()
				storageChannel, _ = s.NewChannel(c.id, cover, storage.WithChannelFields(c.Fields()))
				c.cover.Store(int64(cover))
				c.storageChannel.Store(&storageChannel)
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
				c.Logger().Info("存储通道创建成功", log.Duration("覆盖周期", cover))
			} else {
				// 更改存储覆盖周期
				storageChannel.SetCover(cover)
				c.Logger().Info("存储通道覆盖周期修改成功", log.Duration("覆盖周期", cover))
			}
			return nil
		})
	})
}

func (c *Channel) StopStorage() <-chan error {
	if storageChannel := c.loadStorageChannel(); storageChannel != nil {
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
		c.enableRecord.Store(true)
		c.Logger().Info("启动录像成功")
		return nil
	})
}

func (c *Channel) StopRecord() {
	c.enableRecord.Store(false)
	if storageChannel := c.loadStorageChannel(); storageChannel != nil {
		storageChannel.Sync()
	}
	c.Logger().Info("停止录像成功")
	return
}

func (c *Channel) allocHistoryRTPPusherID() uint64 {
	id := c.historyRTPPusherID
	c.historyRTPPusherID++
	return id
}

func (c *Channel) OpenHistoryRTPPusher(targetIP net.IP, targetPort int, transport string, startTime, endTime int64, ssrc int64,
	eofCallback HistoryRTPPusherEOFCallback, closedCallback HistoryRTPPusherClosedCallback) (*HistoryRTPPusher, error) {
	return lock.RLockGetDouble(c.runner, func() (*HistoryRTPPusher, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开历史音视频RTP推流失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		id := c.allocHistoryRTPPusherID()
		pusher, err := NewHistoryRTPPusher(id, c, targetIP, targetPort, transport, startTime, endTime, ssrc, eofCallback)
		if err != nil {
			return nil, c.Logger().ErrorWith("打开历史音视频RTP推流失败", err)
		}
		pusher.OnClosed(func(l lifecycle.Lifecycle, err error) {
			if closedCallback != nil {
				closedCallback(pusher, c)
			}
			pusher.Logger().Info("历史音视频RTP推流服务已关闭")
			// 已关闭的历史音视频RTP推流将其从通道中删除
			c.historyRTPPushers.Delete(id)
		}).OnStarted(func(l lifecycle.Lifecycle, err error) {
			if err != nil {
				if closedCallback != nil {
					closedCallback(pusher, c)
				}
				// 启动失败的历史音视频RTP推流将其从通道中删除
				c.historyRTPPushers.Delete(id)
			} else {
				pusher.Logger().Info("历史音视频RTP推流服务启动成功")
			}
		}).OnClose(func(l lifecycle.Lifecycle, err error) {
			pusher.Logger().Info("历史音视频RTP推流服务执行关闭操作成功")
		}).Background()
		c.historyRTPPushers.Store(id, pusher)
		c.Logger().Info("历史音视频RTP推流服务创建成功", log.Uint64("id", id))
		return pusher, nil
	})
}

func (c *Channel) StartHistoryRTPPusher(id uint64) error {
	if pusher, _ := c.historyRTPPushers.Load(id); pusher != nil {
		pusher.Push()
		return nil
	}
	return c.Logger().ErrorWith("设置历史音视频RTP推流服务开始推流失败", errPkg.NewNotFound(c.HistoryRTPPlayerDisplayName(id)))
}

func (c *Channel) GetHistoryRTPPusher(id uint64) *HistoryRTPPusher {
	if pusher, _ := c.historyRTPPushers.Load(id); pusher != nil {
		return pusher
	}
	return nil
}

func (c *Channel) PauseHistoryRTPPusher(id uint64) error {
	if pusher, _ := c.historyRTPPushers.Load(id); pusher != nil {
		pusher.Logger().Debug("历史音视频RTP推流服务请求暂停")
		pusher.Pause()
		return nil
	}
	return c.Logger().ErrorWith("暂停历史音视频RTP推流失败", errPkg.NewNotFound(c.HistoryRTPPlayerDisplayName(id)))
}

func (c *Channel) ResumeHistoryRTPPusher(id uint64) error {
	if pusher, _ := c.historyRTPPushers.Load(id); pusher != nil {
		pusher.Logger().Debug("历史音视频RTP推流服务请求恢复")
		pusher.Resume()
		return nil
	}
	return c.Logger().ErrorWith("恢复历史音视频RTP推流失败", errPkg.NewNotFound(c.HistoryRTPPlayerDisplayName(id)))
}

func (c *Channel) SeekHistoryRTPPusher(id uint64, t int64) error {
	if pusher, _ := c.historyRTPPushers.Load(id); pusher != nil {
		pusher.Logger().Debug("历史音视频RTP推流服务请求定位", log.Time("定位时间", time.UnixMilli(t)))
		return pusher.Seek(t)
	}
	return c.Logger().ErrorWith("定位历史音视频RTP推流失败", errPkg.NewNotFound(c.HistoryRTPPlayerDisplayName(id)))
}

func (c *Channel) SetHistoryRTPPusherScale(id uint64, scale float64) error {
	if pusher, _ := c.historyRTPPushers.Load(id); pusher != nil {
		pusher.Logger().Debug("历史音视频RTP推流服务请求设置倍速", log.Float64("倍速", scale))
		return pusher.SetScale(scale)
	}
	return c.Logger().ErrorWith("设置历史音视频RTP推流倍速失败", errPkg.NewNotFound(c.HistoryRTPPlayerDisplayName(id)))
}

func (c *Channel) CloseHistoryRTPPusher(id uint64) <-chan error {
	if pusher, _ := c.historyRTPPushers.Load(id); pusher != nil {
		pusher.Close(nil)
		return pusher.ClosedWaiter()
	}
	return nil
}
