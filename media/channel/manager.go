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

func (m *Manager) Get(id string) *Channel {
	if channel, ok := m.channels.Load(id); ok {
		return channel
	}
	return nil
}

func (m *Manager) Delete(id string) <-chan error {
	if channel := m.Get(id); channel != nil {
		channel.Close(nil)
		return channel.ClosedWaiter()
	}
	return nil
}

func (m *Manager) OpenRtpPlayer(id string, transport string, timeout time.Duration, closedCallback RtpPlayerCloseCallback) (*RtpPlayer, error) {
	if channel := m.Get(id); channel != nil {
		return channel.OpenRtpPlayer(transport, timeout, closedCallback)
	}
	return nil, m.Logger().ErrorWith("打开RTP拉流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) SetupRtpPlayer(id string, rtpMap map[uint8]string, remoteIP net.IP, remotePort int, ssrc int64, once bool) error {
	if channel := m.Get(id); channel != nil {
		return channel.SetupRtpPlayer(rtpMap, remoteIP, remotePort, ssrc, once)
	}
	return m.Logger().ErrorWith("设置RTP拉流参数失败", errors.NewNotFound(m.ChannelDisplayName(id)))

}

func (m *Manager) GetRtpPlayer(id string) (*RtpPlayer, error) {
	if channel := m.Get(id); channel != nil {
		return channel.GetRtpPlayer(), nil
	}
	return nil, m.Logger().ErrorWith("获取RTP拉流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) CloseRtpPlayer(id string) (<-chan error, error) {
	if channel := m.Get(id); channel != nil {
		return channel.CloseRtpPlayer(), nil
	}
	return nil, m.Logger().ErrorWith("关闭RTP拉流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) SetupStorage(id string, cover time.Duration) error {
	if channel := m.Get(id); channel != nil {
		return channel.SetupStorage(cover)
	}
	return m.Logger().ErrorWith("配置存储失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StopStorage(id string) (<-chan error, error) {
	if channel := m.Get(id); channel != nil {
		return channel.StopStorage(), nil
	}
	return nil, m.Logger().ErrorWith("停止存储失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StartRecord(id string) error {
	if channel := m.Get(id); channel != nil {
		return channel.StartRecord()
	}
	return m.Logger().ErrorWith("启动录像失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StopRecord(id string) error {
	if channel := m.Get(id); channel != nil {
		channel.StopRecord()
		return nil
	}
	return m.Logger().ErrorWith("停止录像失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) OpenHistoryRtpPusher(id string, targetIP net.IP, targetPort int, transport string, startTime, endTime int64, ssrc int64,
	eofCallback HistoryRtpPusherEOFCallback, closedCallback HistoryRtpPusherClosedCallback) (*HistoryRtpPusher, error) {
	if channel := m.Get(id); channel != nil {
		return channel.OpenHistoryRtpPusher(targetIP, targetPort, transport, startTime, endTime, ssrc, eofCallback, closedCallback)
	}
	return nil, m.Logger().ErrorWith("打开历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StartHistoryRtpPusher(channelID string, pusherID uint64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.StartHistoryRtpPusher(pusherID)
	}
	return m.Logger().ErrorWith("启动历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))

}

func (m *Manager) GetHistoryRtpPusher(channelID string, pusherID uint64) (*HistoryRtpPusher, error) {
	if channel := m.Get(channelID); channel != nil {
		return channel.GetHistoryRtpPusher(pusherID), nil
	}
	return nil, m.Logger().ErrorWith("获取历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
}

func (m *Manager) PauseHistoryRtpPusher(channelID string, pusherID uint64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.PauseHistoryRtpPusher(pusherID)
	}
	return m.Logger().ErrorWith("暂停历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
}

func (m *Manager) ResumeHistoryRtpPusher(channelID string, pusherID uint64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.ResumeHistoryRtpPusher(pusherID)
	}
	return m.Logger().ErrorWith("恢复历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
}

func (m *Manager) SeekHistoryRtpPusher(channelID string, pusherID uint64, t int64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.SeekHistoryRtpPusher(pusherID, t)
	}
	return m.Logger().ErrorWith("定位历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
}

func (m *Manager) SetHistoryRtpPusherScale(channelID string, pusherID uint64, scale float64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.SetHistoryRtpPusherScale(pusherID, scale)
	}
	return m.Logger().ErrorWith("设置历史音视频RTP推流倍速失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))

}

func (m *Manager) CloseHistoryRtpPusher(channelID string, pusherID uint64) (<-chan error, error) {
	if channel := m.Get(channelID); channel != nil {
		return channel.CloseHistoryRtpPusher(pusherID), nil
	}
	return nil, m.Logger().ErrorWith("关闭历史音视频RTP推流失败", errors.NewNotFound(m.ChannelDisplayName(channelID)))
}

var GetManager = component.NewPointer(NewManager).Get
