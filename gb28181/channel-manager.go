package gb28181

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/container"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/db"
	modelPkg "gitee.com/sy_183/cvds-mas/gb28181/model"
	"gitee.com/sy_183/cvds-mas/gb28181/sip"
	responsePkg "gitee.com/sy_183/cvds-mas/gb28181/sip/response"
	mediaChannel "gitee.com/sy_183/cvds-mas/media/channel"
	"net/http"
)

const (
	ChannelManagerModule     = Module + ".channel-manager"
	ChannelManagerModuleName = "国标通道管理器"
)

const MaxChannel = 1024

type ChannelManager struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	dbManager            *db.DBManager
	mcManager            *mediaChannel.Manager
	storageConfigManager *StorageConfigManager

	channels container.SyncMap[string, *Channel]

	httpClient http.Client

	log.AtomicLogger
}

func newChannelManager() *ChannelManager {
	m := &ChannelManager{
		dbManager: GetDBManager(),
		mcManager: mediaChannel.GetManager(),
	}

	config.InitModuleLogger(m, ChannelManagerModule, ChannelManagerModuleName)
	config.RegisterLoggerConfigReloadedCallback(m, ChannelManagerModule, ChannelManagerModuleName)
	m.runner = lifecycle.NewWithInterruptedRun(m.start, m.run)
	m.Lifecycle = m.runner
	return m
}

func (m *ChannelManager) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	m.storageConfigManager = GetStorageConfigManager()

	var channels []modelPkg.Channel
	if res := m.dbManager.Table(modelPkg.ChannelTableName).Limit(MaxChannel).Find(&channels); res.Error != nil {
		m.Logger().ErrorWith("加载通道信息失败", res.Error)
	} else {
		m.Logger().Info("加载通道信息成功", log.Int("通道数量", len(channels)))
	}

	for i := range channels {
		channel := newChannel(&channels[i], m)
		if _, exist := m.channels.LoadOrStore(channel.Name(), channel); exist {
			m.Logger().Error("通道名称重复，忽略此通道", log.String("通道名称", channel.Name()))
			continue
		}
		const loggerConfigReloadedCallbackId = "loggerConfigReloadedCallbackId"
		config.InitModuleLogger(channel, ChannelModule, channel.DisplayName())
		channel.SetField(loggerConfigReloadedCallbackId, config.RegisterLoggerConfigReloadedCallback(channel, ChannelModule, channel.DisplayName()))
		channel.OnClosed(func(l lifecycle.Lifecycle, err error) {
			channel.Logger().Info("通道已关闭")
			m.channels.Delete(channel.Name())
			config.UnregisterLoggerConfigReloadedCallback(channel.Field(loggerConfigReloadedCallbackId).(uint64))
		}).OnStarted(func(l lifecycle.Lifecycle, err error) {
			if err != nil {
				m.channels.Delete(channel.Name())
				config.UnregisterLoggerConfigReloadedCallback(channel.Field(loggerConfigReloadedCallbackId).(uint64))
			} else {
				channel.Logger().Info("通道启动成功")
			}
		}).Background()
		m.Logger().Info("通道创建成功", log.String("通道名称", channel.Name()))
	}

	return nil
}

func (m *ChannelManager) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	<-interrupter

	futures := make([]<-chan error, 0)
	m.channels.Range(func(name string, ch *Channel) bool {
		ch.Close(nil)
		futures = append(futures, ch.ClosedWaiter())
		return true
	})
	for _, future := range futures {
		<-future
	}

	return nil
}

func (m *ChannelManager) GetChannel(name string) *modelPkg.Channel {
	if channel, exist := m.channels.Load(name); exist {
		return channel.Model()
	}
	return nil
}

func (m *ChannelManager) AddChannel(channel *modelPkg.Channel) error {
	return nil
}

func (m *ChannelManager) DeleteChannel(name string) error {
	return nil
}

func (m *ChannelManager) createResponse(request *sip.Request, statusCode int, toTag string) *sip.Response {
	response := &sip.Response{
		StatusCode:   statusCode,
		ReasonPhrase: responsePkg.ReasonPhrase(statusCode),
		Message: sip.Message{
			LocalIp:   request.LocalIp,
			LocalPort: request.LocalPort,
			From:      request.From,
			To:        request.To,
			Via:       append([]sip.Via(nil), request.Via...),
			CSeq:      request.CSeq,
			CallId:    request.CallId,
		},
	}
	if response.To.Tag == "" {
		if toTag != "" {
			response.To.Tag = toTag
		} else {
			response.To.Tag = sip.CreateTag()
		}
	}
	return response
}

func (m *ChannelManager) ProcessInvite(request *sip.Request) *sip.Response {
	toURI := new(sip.URI)
	if err := toURI.Parse(request.To.Address.URI); err != nil {
		return m.createResponse(request, responsePkg.BadRequest, "")
	}

	channel, exist := m.channels.Load(toURI.User)
	if exist {
		return channel.ProcessProxyInvite(request)
	}
	return m.createResponse(request, responsePkg.NotFound, "")
}

func (m *ChannelManager) ProcessAck(request *sip.Request) {
	toURI := new(sip.URI)
	if err := toURI.Parse(request.To.Address.URI); err != nil {
		return
	}

	channel, exist := m.channels.Load(toURI.User)
	if exist {
		channel.ProcessProxyAck(request)
	}
}

func (m *ChannelManager) ProcessBye(request *sip.Request) *sip.Response {
	toURI := new(sip.URI)
	if err := toURI.Parse(request.To.Address.URI); err != nil {
		return m.createResponse(request, responsePkg.BadRequest, "")
	}

	proxyConfig := config.GB28181ProxyConfig()
	if toURI.User == proxyConfig.Id {
		fromURI := new(sip.URI)
		if err := toURI.Parse(request.From.Address.URI); err != nil {
			return m.createResponse(request, responsePkg.BadRequest, "")
		}
		channel, exist := m.channels.Load(fromURI.User)
		if exist {
			return channel.ProcessBye(request)
		}
		return m.createResponse(request, responsePkg.CallOrTransactionDoesNotExist, "")
	}

	channel, exist := m.channels.Load(toURI.User)
	if exist {
		return channel.ProcessProxyBye(request)
	}
	return m.createResponse(request, responsePkg.CallOrTransactionDoesNotExist, "")
}

func (m *ChannelManager) ProcessInfo(request *sip.Request) *sip.Response {
	toURI := new(sip.URI)
	if err := toURI.Parse(request.To.Address.URI); err != nil {
		return m.createResponse(request, responsePkg.BadRequest, "")
	}

	channel, exist := m.channels.Load(toURI.User)
	if exist {
		return channel.ProcessProxyInfo(request)
	}
	return m.createResponse(request, responsePkg.NotFound, "")
}

var GetChannelManager = component.NewPointer(newChannelManager).Get
