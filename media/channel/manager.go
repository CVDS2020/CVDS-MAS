package channel

import (
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/media"
	"net"
	"sync"
	"time"
)

const (
	ManagerModule     = Module + ".manager"
	ManagerModuleName = "媒体通道管理器"
)

var ManagerClosedError = errors.New("通道管理器已经关闭或正在关闭")

type ChannelNotFoundError struct {
	ID string
}

func (e *ChannelNotFoundError) Error() string {
	return fmt.Sprintf("通道(%s)不存在", e.ID)
}

type Manager struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	channels    container.SyncMap[string, *Channel]
	channelLock sync.Mutex

	log.AtomicLogger
}

func NewManager() *Manager {
	m := new(Manager)
	config.InitModuleLogger(m, ManagerModule, ManagerModuleName)
	config.RegisterLoggerConfigReloadedCallback(m, ManagerModule, ManagerModuleName)
	m.runner = lifecycle.NewWithInterruptedRun(nil, m.run)
	m.Lifecycle = m.runner
	return m
}

func (m *Manager) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	<-interrupter
	lock.LockDo(m.runner, func() {})
	var futures []<-chan error
	m.channels.Range(func(id string, channel *Channel) bool {
		channel.Close(nil)
		futures = append(futures, channel.ClosedWaiter())
		return true
	})
	for _, future := range futures {
		<-future
	}
	return nil
}

func (m *Manager) ChannelDisplayName(id string) string {
	return fmt.Sprintf("媒体通道(%s)", id)
}

func (m *Manager) Create(id string, options ...Option) (*Channel, error) {
	return lock.RLockGetDouble(m.runner, func() (*Channel, error) {
		if !m.runner.Running() {
			return nil, m.Logger().ErrorWith("通道创建失败", lifecycle.NewStateNotRunningError(ManagerModuleName))
		}

		channel := NewChannel(id, options...)
		if old, hasOld := m.channels.LoadOrStore(id, channel); hasOld {
			return old, errors.NewExist(m.ChannelDisplayName(id))
		}
		const loggerConfigReloadedCallbackId = "loggerConfigReloadedCallbackId"
		config.InitModuleLogger(channel, ChannelModule, channel.DisplayName())
		channel.SetField(loggerConfigReloadedCallbackId, config.RegisterLoggerConfigReloadedCallback(channel, ChannelModule, channel.DisplayName()))
		channel.OnClosed(func(l lifecycle.Lifecycle, err error) {
			channel.Logger().Info("通道已关闭")
			m.channels.Delete(id)
			config.UnregisterLoggerConfigReloadedCallback(channel.Field(loggerConfigReloadedCallbackId).(uint64))
		})
		channel.Start()
		m.Logger().Info("通道创建成功", log.String("通道ID", id))

		return channel, nil
	})
}

func (m *Manager) Get(id string) (*Channel, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel, nil
	}
	return nil, errors.NewNotFound(m.ChannelDisplayName(id))
}

func (m *Manager) Delete(id string) (<-chan error, error) {
	if channel, ok := m.channels.Load(id); ok {
		channel.Close(nil)
		return channel.ClosedWaiter(), nil
	}
	return nil, errors.NewNotFound(m.ChannelDisplayName(id))
}

func (m *Manager) OpenRtpPlayer(id string, closedCallback RtpPlayerCloseCallback) (*RtpPlayer, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.OpenRtpPlayer(closedCallback)
	}
	return nil, m.Logger().ErrorWith("打开RTP拉流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) GetRtpPlayer(id string) (*RtpPlayer, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.GetRtpPlayer()
	}
	return nil, m.Logger().ErrorWith("获取RTP拉流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) RtpPlayerAddStream(id string, transport string) (streamPlayer *RtpStreamPlayer, err error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.RtpPlayerAddStream(transport)
	}
	return nil, m.Logger().ErrorWith("RTP拉流通道添加失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) RtpPlayerSetupStream(id string, streamId uint64, remoteIP net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo, config SetupConfig) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.RtpPlayerSetupStream(streamId, remoteIP, remotePort, ssrc, streamInfo, config)
	}
	return m.Logger().ErrorWith("RTP拉流通道设置失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) RtpPlayerRemoveStream(id string, streamId uint64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.RtpPlayerRemoveStream(streamId)
	}
	return m.Logger().ErrorWith("RTP拉流通道移除失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) CloseRtpPlayer(id string) (<-chan error, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.CloseRtpPlayer()
	}
	return nil, m.Logger().ErrorWith("关闭RTP拉流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) SetupStorage(id string, cover time.Duration, bufferSize uint) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.SetupStorage(cover, bufferSize)
	}
	return m.Logger().ErrorWith("配置存储服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StopStorage(id string) (<-chan error, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.StopStorage()
	}
	return nil, m.Logger().ErrorWith("停止存储服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StartRecord(id string) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.StartRecord()
	}
	return m.Logger().ErrorWith("启动录像失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StopRecord(id string) error {
	if channel, ok := m.channels.Load(id); ok {
		channel.StopRecord()
		return nil
	}
	return m.Logger().ErrorWith("停止录像失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) OpenRtpPusher(id string, maxCached int, closedCallback RtpPusherClosedCallback) (*RtpPusher, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.OpenRtpPusher(maxCached, closedCallback)
	}
	return nil, m.Logger().ErrorWith("打开实时音视频RTP推流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) GetRtpPusher(id string, pusherId uint64) (*RtpPusher, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.GetRtpPusher(pusherId)
	}
	return nil, m.Logger().ErrorWith("获取实时音视频RTP推流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) RtpPushers(id string) ([]*RtpPusher, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.RtpPushers(), nil
	}
	return nil, m.Logger().ErrorWith("获取所有实时音视频RTP推流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) RtpPusherAddStream(id string, pusherId uint64, transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (streamPusher *RtpStreamPusher, err error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.RtpPusherAddStream(pusherId, transport, remoteIp, remotePort, ssrc, streamInfo)
	}
	return nil, m.Logger().ErrorWith("实时音视频RTP推流通道添加失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) RtpPusherStartPush(id string, pusherId uint64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.RtpPusherStartPush(pusherId)
	}
	return m.Logger().ErrorWith("实时音视频RTP推流服务开始推流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) RtpPusherPause(id string, pusherId uint64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.RtpPusherPause(pusherId)
	}
	return m.Logger().ErrorWith("实时音视频RTP推流服务暂停推流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) RtpPusherRemoveStream(id string, pusherId uint64, streamId uint64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.RtpPusherRemoveStream(pusherId, streamId)
	}
	return m.Logger().ErrorWith("实时音视频RTP推流通道移除失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) CloseRtpPusher(id string, pusherId uint64) (<-chan error, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.CloseRtpPusher(pusherId)
	}
	return nil, m.Logger().ErrorWith("关闭实时音视频RTP推流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) OpenHistoryRtpPusher(id string, startTime, endTime int64, eofCallback HistoryRtpPusherEOFCallback, closedCallback HistoryRtpPusherClosedCallback) (*HistoryRtpPusher, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.OpenHistoryRtpPusher(startTime, endTime, eofCallback, closedCallback)
	}
	return nil, m.Logger().ErrorWith("打开历史音视频RTP推流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) GetHistoryRtpPusher(id string, pusherId uint64) (*HistoryRtpPusher, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.GetHistoryRtpPusher(pusherId)
	}
	return nil, m.Logger().ErrorWith("获取历史音视频RTP推流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) HistoryRtpPushers(id string) ([]*HistoryRtpPusher, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.HistoryRtpPushers(), nil
	}
	return nil, m.Logger().ErrorWith("获取所有历史音视频RTP推流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) HistoryRtpPusherAddStream(id string, pusherId uint64, transport string, remoteIp net.IP, remotePort int, ssrc int64, streamInfo *media.RtpStreamInfo) (streamPusher *RtpStreamPusher, err error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.HistoryRtpPusherAddStream(pusherId, transport, remoteIp, remotePort, ssrc, streamInfo)
	}
	return nil, m.Logger().ErrorWith("历史音视频RTP推流通道添加失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) HistoryRtpPusherStartPush(id string, pusherId uint64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.HistoryRtpPusherStartPush(pusherId)
	}
	return m.Logger().ErrorWith("历史音视频RTP推流服务开始推流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) HistoryRtpPusherPause(id string, pusherId uint64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.HistoryRtpPusherPause(pusherId)
	}
	return m.Logger().ErrorWith("历史音视频RTP推流服务暂停推流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) HistoryRtpPusherSeek(id string, pusherId uint64, t int64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.HistoryRtpPusherSeek(pusherId, t)
	}
	return m.Logger().ErrorWith("历史音视频RTP推流服务定位失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) HistoryRtpPusherSetScale(id string, pusherId uint64, scale float64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.HistoryRtpPusherSetScale(pusherId, scale)
	}
	return m.Logger().ErrorWith("历史音视频RTP推流服务设置倍速失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) HistoryRtpPusherRemoveStream(id string, pusherId uint64, streamId uint64) error {
	if channel, ok := m.channels.Load(id); ok {
		return channel.HistoryRtpPusherRemoveStream(pusherId, streamId)
	}
	return m.Logger().ErrorWith("历史音视频RTP推流通道移除失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) CloseHistoryRtpPusher(id string, pusherId uint64) (<-chan error, error) {
	if channel, ok := m.channels.Load(id); ok {
		return channel.CloseHistoryRtpPusher(pusherId)
	}
	return nil, m.Logger().ErrorWith("关闭历史音视频RTP推流服务失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

//func (m *Manager) OpenHistoryRtpPusher(id string, targetIP net.IP, targetPort int, transport string, startTime, endTime int64, ssrc int64,
//	eofCallback HistoryRtpPusherEOFCallback, closedCallback HistoryRtpPusherClosedCallback) (*HistoryRtpPusher, error) {
//	if channel := m.Get(id); channel != nil {
//		return channel.OpenHistoryRtpPusher(targetIP, targetPort, transport, startTime, endTime, ssrc, eofCallback, closedCallback)
//	}
//	return nil, m.Logger().ErrorWith("打开历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
//}
//
//func (m *Manager) StartHistoryRtpPusher(channelID string, pusherID uint64) error {
//	if channel := m.Get(channelID); channel != nil {
//		return channel.StartHistoryRtpPusher(pusherID)
//	}
//	return m.Logger().ErrorWith("启动历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
//
//}
//
//func (m *Manager) GetHistoryRtpPusher(channelID string, pusherID uint64) (*HistoryRtpPusher, error) {
//	if channel := m.Get(channelID); channel != nil {
//		return channel.GetHistoryRtpPusher(pusherID), nil
//	}
//	return nil, m.Logger().ErrorWith("获取历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
//}
//
//func (m *Manager) PauseHistoryRtpPusher(channelID string, pusherID uint64) error {
//	if channel := m.Get(channelID); channel != nil {
//		return channel.PauseHistoryRtpPusher(pusherID)
//	}
//	return m.Logger().ErrorWith("暂停历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
//}
//
//func (m *Manager) ResumeHistoryRtpPusher(channelID string, pusherID uint64) error {
//	if channel := m.Get(channelID); channel != nil {
//		return channel.ResumeHistoryRtpPusher(pusherID)
//	}
//	return m.Logger().ErrorWith("恢复历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
//}
//
//func (m *Manager) SeekHistoryRtpPusher(channelID string, pusherID uint64, t int64) error {
//	if channel := m.Get(channelID); channel != nil {
//		return channel.SeekHistoryRtpPusher(pusherID, t)
//	}
//	return m.Logger().ErrorWith("定位历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
//}
//
//func (m *Manager) SetHistoryRtpPusherScale(channelID string, pusherID uint64, scale float64) error {
//	if channel := m.Get(channelID); channel != nil {
//		return channel.SetHistoryRtpPusherScale(pusherID, scale)
//	}
//	return m.Logger().ErrorWith("设置历史音视频RTP推流倍速失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
//
//}
//
//func (m *Manager) CloseHistoryRtpPusher(channelID string, pusherID uint64) (<-chan error, error) {
//	if channel := m.Get(channelID); channel != nil {
//		return channel.CloseHistoryRtpPusher(pusherID), nil
//	}
//	return nil, m.Logger().ErrorWith("关闭历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
//}

var GetManager = component.NewPointer(NewManager).Get
