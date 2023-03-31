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

	id          string
	storageType string

	rtpPlayer     atomic.Pointer[RtpPlayer]
	rtpPlayerLock sync.RWMutex

	storageChannel     atomic.Pointer[storage.Channel]
	storageChannelLock sync.Mutex
	enableRecord       atomic.Bool

	rtpPushers     container.SyncMap[uint64, *RtpPusher]
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

func (c *Channel) StorageDisplayName() string {
	return fmt.Sprintf("媒体通道(%s)存储服务", c.id)
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

func (c *Channel) StorageType() string {
	return c.storageType
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
		"id":          c.ID(),
		"storageType": c.StorageType(),
		"fields":      c.Fields(),
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
	var rtpPusherFutures []<-chan error
	c.rtpPushers.Range(func(id uint64, pusher *RtpPusher) bool {
		pusher.Close(nil)
		rtpPusherFutures = append(rtpPusherFutures, pusher.ClosedWaiter())
		return true
	})
	for _, future := range rtpPusherFutures {
		<-future
	}
	if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
		rtpPlayer.Shutdown()
	}
	if storageChannel := c.StorageChannel(); storageChannel != nil {
		storageChannel.Shutdown()
	}
	return nil
}

func (c *Channel) OpenRtpPlayer(closedCallback RtpPlayerCloseCallback) (*RtpPlayer, error) {
	return lock.RLockGetDouble(c.runner, func() (*RtpPlayer, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开RTP拉流失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		return lock.LockGetDouble(&c.rtpPlayerLock, func() (*RtpPlayer, error) {
			if player := c.rtpPlayer.Load(); player != nil {
				return player, errors.NewExist(c.RtpPlayerDisplayName())
			}
			player := newRtpPlayer(c)
			player.OnClosed(func(l lifecycle.Lifecycle, err error) {
				if closedCallback != nil {
					closedCallback(player, c)
				}
				player.Logger().Info("RTP拉流服务已关闭")
				lock.LockDo(&c.rtpPlayerLock, func() { c.rtpPlayer.Store(nil) })
			}).OnClose(func(l lifecycle.Lifecycle, err error) {
				player.Logger().Info("RTP拉流服务执行关闭操作成功")
			}).Start()

			c.rtpPlayer.Store(player)
			return player, nil
		})
	})
}

func (c *Channel) GetRtpPlayer() (*RtpPlayer, error) {
	if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
		return rtpPlayer, nil
	}
	return nil, c.Logger().ErrorWith("获取RTP拉流服务失败", errors.NewNotFound(c.RtpPlayerDisplayName()))
}

func (c *Channel) RtpPlayerAddStream(transport string) (streamPlayer *RtpStreamPlayer, err error) {
	if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
		return rtpPlayer.AddStream(transport)
	}
	return nil, c.Logger().ErrorWith("RTP拉流通道添加失败", errors.NewNotFound(c.RtpPlayerDisplayName()))
}

func (c *Channel) RtpPlayerSetupStream(streamId uint64, remoteIP net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo, config SetupConfig) error {
	if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
		return rtpPlayer.SetupStream(streamId, remoteIP, remotePort, ssrc, streamInfo, config)
	}
	return c.Logger().ErrorWith("RTP拉流通道设置失败", errors.NewNotFound(c.RtpPlayerDisplayName()))
}

func (c *Channel) RtpPlayerRemoveStream(streamId uint64) error {
	if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
		return rtpPlayer.RemoveStream(streamId)
	}
	return c.Logger().ErrorWith("RTP拉流通道移除失败", errors.NewNotFound(c.RtpPlayerDisplayName()))
}

func (c *Channel) CloseRtpPlayer() (<-chan error, error) {
	if rtpPlayer := c.rtpPlayer.Load(); rtpPlayer != nil {
		rtpPlayer.Close(nil)
		return rtpPlayer.ClosedWaiter(), nil
	}
	return nil, c.Logger().ErrorWith("关闭RTP拉流服务失败", errors.NewNotFound(c.RtpPlayerDisplayName()))
}

func (c *Channel) SetupStorage(cover time.Duration, bufferSize uint) error {
	return lock.RLockGet(c.runner, func() error {
		if !c.runner.Running() {
			return c.Logger().ErrorWith("设置存储配置失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}

		return lock.LockGet(&c.storageChannelLock, func() error {
			if storageChannel := c.StorageChannel(); storageChannel == nil {
				// 存储通道还未创建，创建并启动存储通道
				s := fileStorage.GetStorage()
				storageChannel, _ = s.NewChannel(c.id, cover,
					fileStorage.WithWriteBufferPoolConfig(bufferSize, pool.StackPoolProvider[*fileStorage.Buffer](2)))
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

func (c *Channel) StopStorage() (<-chan error, error) {
	if storageChannel := c.StorageChannel(); storageChannel != nil {
		storageChannel.Close(nil)
		return storageChannel.ClosedWaiter(), nil
	}
	return nil, c.Logger().ErrorWith("停止存储服务失败", errors.NewNotFound(c.StorageDisplayName()))
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
	c.rtpPushersId++
	return c.rtpPushersId
}

func (c *Channel) OpenRtpPusher(maxCached int, closedCallback RtpPusherClosedCallback) (*RtpPusher, error) {
	return lock.RLockGetDouble(c.runner, func() (*RtpPusher, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开实时音视频RTP推流服务失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		id := c.allocRtpPusherId()
		pusher := newRtpPusher(id, c, maxCached)
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
		c.rtpPushers.Store(id, pusher)
		c.Logger().Info("实时音视频RTP推流服务创建成功", log.Uint64("id", id))
		return pusher, nil
	})
}

func (c *Channel) GetRtpPusher(id uint64) (*RtpPusher, error) {
	if rtpPusher, _ := c.rtpPushers.Load(id); rtpPusher != nil {
		return rtpPusher, nil
	}
	return nil, c.Logger().ErrorWith("获取实时音视频RTP推流服务失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) RtpPushers() []*RtpPusher {
	return c.rtpPushers.Values()
}

func (c *Channel) RtpPusherAddStream(id uint64, transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (streamPusher *RtpStreamPusher, err error) {
	if rtpPusher, _ := c.rtpPushers.Load(id); rtpPusher != nil {
		return rtpPusher.AddStream(transport, remoteIp, remotePort, ssrc, streamInfo)
	}
	return nil, c.Logger().ErrorWith("实时音视频RTP推流通道添加失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) RtpPusherStartPush(id uint64) error {
	if rtpPusher, _ := c.rtpPushers.Load(id); rtpPusher != nil {
		rtpPusher.StartPush()
		return nil
	}
	return c.Logger().ErrorWith("实时音视频RTP推流服务开始推流失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) RtpPusherPause(id uint64) error {
	if rtpPusher, _ := c.rtpPushers.Load(id); rtpPusher != nil {
		rtpPusher.Pause()
		return nil
	}
	return c.Logger().ErrorWith("实时音视频RTP推流服务暂停推流失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) RtpPusherRemoveStream(id uint64, streamId uint64) error {
	if rtpPusher, _ := c.rtpPushers.Load(id); rtpPusher != nil {
		return rtpPusher.RemoveStream(streamId)
	}
	return c.Logger().ErrorWith("实时音视频RTP推流通道移除失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) CloseRtpPusher(id uint64) (<-chan error, error) {
	if rtpPusher, _ := c.rtpPushers.Load(id); rtpPusher != nil {
		rtpPusher.Close(nil)
		return rtpPusher.ClosedWaiter(), nil
	}
	return nil, c.Logger().ErrorWith("关闭实时音视频RTP推流服务失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) allocHistoryRtpPusherID() uint64 {
	c.historyRtpPusherId++
	return c.historyRtpPusherId
}

func (c *Channel) OpenHistoryRtpPusher(startTime, endTime int64, eofCallback HistoryRtpPusherEOFCallback, closedCallback HistoryRtpPusherClosedCallback) (*HistoryRtpPusher, error) {
	return lock.RLockGetDouble(c.runner, func() (*HistoryRtpPusher, error) {
		if !c.runner.Running() {
			return nil, c.Logger().ErrorWith("打开历史音视频RTP推流服务失败", lifecycle.NewStateNotRunningError(c.DisplayName()))
		}
		id := c.allocHistoryRtpPusherID()
		pusher, err := newHistoryRtpPusher(id, c, startTime, endTime, eofCallback)
		if err != nil {
			return nil, c.Logger().ErrorWith("打开历史音视频RTP推流服务失败", err)
		}
		pusher.OnClosed(func(l lifecycle.Lifecycle, err error) {
			if closedCallback != nil {
				closedCallback(pusher, c)
			}
			pusher.Logger().Info("历史音视频RTP推流服务已关闭")
			// 已关闭的历史音视频RTP推流服务将其从通道中删除
			c.historyRtpPushers.Delete(id)
		}).OnClose(func(l lifecycle.Lifecycle, err error) {
			pusher.Logger().Info("历史音视频RTP推流服务执行关闭操作成功")
		}).Start()
		c.historyRtpPushers.Store(id, pusher)
		c.Logger().Info("历史音视频RTP推流服务创建成功", log.Uint64("id", id))
		return pusher, nil
	})
}

func (c *Channel) GetHistoryRtpPusher(id uint64) (*HistoryRtpPusher, error) {
	if rtpPusher, _ := c.historyRtpPushers.Load(id); rtpPusher != nil {
		return rtpPusher, nil
	}
	return nil, c.Logger().ErrorWith("获取历史音视频RTP推流服务失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}

func (c *Channel) HistoryRtpPushers() []*HistoryRtpPusher {
	return c.historyRtpPushers.Values()
}

func (c *Channel) HistoryRtpPusherAddStream(id uint64, transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (streamPusher *RtpStreamPusher, err error) {
	if rtpPusher, _ := c.historyRtpPushers.Load(id); rtpPusher != nil {
		return rtpPusher.AddStream(transport, remoteIp, remotePort, ssrc, streamInfo)
	}
	return nil, c.Logger().ErrorWith("历史音视频RTP推流通道添加失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}

func (c *Channel) HistoryRtpPusherStartPush(id uint64) error {
	if rtpPusher, _ := c.historyRtpPushers.Load(id); rtpPusher != nil {
		rtpPusher.StartPush()
		return nil
	}
	return c.Logger().ErrorWith("历史音视频RTP推流服务开始推流失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) HistoryRtpPusherPause(id uint64) error {
	if rtpPusher, _ := c.historyRtpPushers.Load(id); rtpPusher != nil {
		rtpPusher.Pause()
		return nil
	}
	return c.Logger().ErrorWith("历史音视频RTP推流服务暂停推流失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) HistoryRtpPusherSeek(id uint64, t int64) error {
	if rtpPusher, _ := c.historyRtpPushers.Load(id); rtpPusher != nil {
		return rtpPusher.Seek(t)
	}
	return c.Logger().ErrorWith("历史音视频RTP推流服务定位失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) HistoryRtpPusherSetScale(id uint64, scale float64) error {
	if rtpPusher, _ := c.historyRtpPushers.Load(id); rtpPusher != nil {
		return rtpPusher.SetScale(scale)
	}
	return c.Logger().ErrorWith("历史音视频RTP推流服务设置倍速失败", errors.NewNotFound(c.RtpPusherDisplayName(id)))
}

func (c *Channel) HistoryRtpPusherRemoveStream(id uint64, streamId uint64) error {
	if rtpPusher, _ := c.historyRtpPushers.Load(id); rtpPusher != nil {
		return rtpPusher.RemoveStream(streamId)
	}
	return c.Logger().ErrorWith("历史音视频RTP推流通道移除失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}

func (c *Channel) CloseHistoryRtpPusher(id uint64) (<-chan error, error) {
	if rtpPusher, _ := c.historyRtpPushers.Load(id); rtpPusher != nil {
		rtpPusher.Close(nil)
		return rtpPusher.ClosedWaiter(), nil
	}
	return nil, c.Logger().ErrorWith("关闭历史音视频RTP推流服务失败", errors.NewNotFound(c.HistoryRtpPlayerDisplayName(id)))
}
