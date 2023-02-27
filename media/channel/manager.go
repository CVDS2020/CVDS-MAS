package channel

import (
	"errors"
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	errPkg "gitee.com/sy_183/cvds-mas/errors"
	"net"
	"sync"
	"time"
)

const (
	ManagerModule     = "media.channel.manager"
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

func (m *Manager) Create(id string, fields map[string]any) (*Channel, error) {
	return lock.RLockGetDouble(m.runner, func() (*Channel, error) {
		if !m.runner.Running() {
			return nil, m.Logger().ErrorWith("通道创建失败", lifecycle.NewStateNotRunningError(ManagerModuleName))
		}

		channel := NewChannel(id, WithFields(fields))
		if _, hasOld := m.channels.LoadOrStore(id, channel); hasOld {
			return nil, m.Logger().ErrorWith("通道创建失败", fmt.Errorf("通道(%s)已经存在", id))
		}
		channel.OnClosed(func(l lifecycle.Lifecycle, err error) {
			channel.Logger().Info("通道已关闭")
			m.channels.Delete(id)
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

func (m *Manager) OpenRTPPlayer(id string, transport string, timeout time.Duration, closedCallback RTPPlayerCloseCallback) (*RTPPlayer, error) {
	if channel := m.Get(id); channel != nil {
		return channel.OpenRTPPlayer(transport, timeout, closedCallback)
	}
	return nil, m.Logger().ErrorWith("打开RTP拉流失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) SetupRTPPlayer(id string, rtpMap map[uint8]string, remoteIP net.IP, remotePort int, ssrc int64, once bool) error {
	if channel := m.Get(id); channel != nil {
		return channel.SetupRTPPlayer(rtpMap, remoteIP, remotePort, ssrc, once)
	}
	return m.Logger().ErrorWith("设置RTP拉流参数失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))

}

func (m *Manager) GetRTPPlayer(id string) (*RTPPlayer, error) {
	if channel := m.Get(id); channel != nil {
		return channel.GetRTPPlayer(), nil
	}
	return nil, m.Logger().ErrorWith("获取RTP拉流失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) CloseRTPPlayer(id string) (<-chan error, error) {
	if channel := m.Get(id); channel != nil {
		return channel.CloseRTPPlayer(), nil
	}
	return nil, m.Logger().ErrorWith("关闭RTP拉流失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) SetupStorage(id string, cover time.Duration) error {
	if channel := m.Get(id); channel != nil {
		return channel.SetupStorage(cover)
	}
	return m.Logger().ErrorWith("配置存储失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StopStorage(id string) (<-chan error, error) {
	if channel := m.Get(id); channel != nil {
		return channel.StopStorage(), nil
	}
	return nil, m.Logger().ErrorWith("停止存储失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StartRecord(id string) error {
	if channel := m.Get(id); channel != nil {
		return channel.StartRecord()
	}
	return m.Logger().ErrorWith("启动录像失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StopRecord(id string) error {
	if channel := m.Get(id); channel != nil {
		channel.StopRecord()
		return nil
	}
	return m.Logger().ErrorWith("停止录像失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) OpenHistoryRTPPusher(id string, targetIP net.IP, targetPort int, transport string, startTime, endTime int64, ssrc int64,
	eofCallback HistoryRTPPusherEOFCallback, closedCallback HistoryRTPPusherClosedCallback) (*HistoryRTPPusher, error) {
	if channel := m.Get(id); channel != nil {
		return channel.OpenHistoryRTPPusher(targetIP, targetPort, transport, startTime, endTime, ssrc, eofCallback, closedCallback)
	}
	return nil, m.Logger().ErrorWith("打开历史音视频RTP推流失败", errPkg.NewNotFound(m.ChannelDisplayName(id)))
}

func (m *Manager) StartHistoryRTPPusher(channelID string, pusherID uint64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.StartHistoryRTPPusher(pusherID)
	}
	return m.Logger().ErrorWith("启动历史音视频RTP推流失败", errPkg.NewNotFound(m.ChannelDisplayName(channelID)))

}

func (m *Manager) GetHistoryRTPPusher(channelID string, pusherID uint64) (*HistoryRTPPusher, error) {
	if channel := m.Get(channelID); channel != nil {
		return channel.GetHistoryRTPPusher(pusherID), nil
	}
	return nil, m.Logger().ErrorWith("获取历史音视频RTP推流失败", errPkg.NewNotFound(m.ChannelDisplayName(channelID)))
}

func (m *Manager) PauseHistoryRTPPusher(channelID string, pusherID uint64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.PauseHistoryRTPPusher(pusherID)
	}
	return m.Logger().ErrorWith("暂停历史音视频RTP推流失败", errPkg.NewNotFound(m.ChannelDisplayName(channelID)))
}

func (m *Manager) ResumeHistoryRTPPusher(channelID string, pusherID uint64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.ResumeHistoryRTPPusher(pusherID)
	}
	return m.Logger().ErrorWith("恢复历史音视频RTP推流失败", errPkg.NewNotFound(m.ChannelDisplayName(channelID)))
}

func (m *Manager) SeekHistoryRTPPusher(channelID string, pusherID uint64, t int64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.SeekHistoryRTPPusher(pusherID, t)
	}
	return m.Logger().ErrorWith("定位历史音视频RTP推流失败", errPkg.NewNotFound(m.ChannelDisplayName(channelID)))
}

func (m *Manager) SetHistoryRTPPusherScale(channelID string, pusherID uint64, scale float64) error {
	if channel := m.Get(channelID); channel != nil {
		return channel.SetHistoryRTPPusherScale(pusherID, scale)
	}
	return m.Logger().ErrorWith("设置历史音视频RTP推流倍速失败", errPkg.NewNotFound(m.ChannelDisplayName(channelID)))

}

func (m *Manager) CloseHistoryRTPPusher(channelID string, pusherID uint64) (<-chan error, error) {
	if channel := m.Get(channelID); channel != nil {
		return channel.CloseHistoryRTPPusher(pusherID), nil
	}
	return nil, m.Logger().ErrorWith("关闭历史音视频RTP推流失败", errPkg.NewNotFound(m.ChannelDisplayName(channelID)))
}

var GetManager = component.NewPointer(NewManager).Get
