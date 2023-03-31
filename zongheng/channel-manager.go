package zongheng

import (
	"fmt"
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/log"
	timerPkg "gitee.com/sy_183/common/timer"
	"gitee.com/sy_183/common/utils"
	mapUtils "gitee.com/sy_183/common/utils/map"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/media"
	mediaChannel "gitee.com/sy_183/cvds-mas/media/channel"
	"gitee.com/sy_183/cvds-mas/storage"
	fileStorage "gitee.com/sy_183/cvds-mas/storage/file"
	"gitee.com/sy_183/cvds-mas/zongheng/endpoint"
	modelPkg "gitee.com/sy_183/cvds-mas/zongheng/model"
	syncPkg "sync"
	"time"
)

const (
	ChannelManagerModule     = "zongheng.channel-manager"
	ChannelManagerModuleName = "纵横通道管理器"
)

const (
	MaxDevice              = 100
	MaxDeviceChannel       = 100
	MaxSuperviseTarget     = 100
	MaxSuperviseTargetType = 3
)

type Channel struct {
	mc    *mediaChannel.Channel
	model *modelPkg.DeviceChannel
}

type ChannelManager struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	endpoint  *endpoint.Endpoint
	dbManager *db.DBManager
	mcManager *mediaChannel.Manager

	channels map[string]*Channel

	checkInterval time.Duration
	syncInterval  time.Duration
	syncSignal    chan struct{}

	storageBufferSize uint

	log.AtomicLogger
}

func NewChannelManager() *ChannelManager {
	cfg := config.ZongHengConfig()
	checkInterval := cfg.CheckInterval
	if checkInterval != 0 && checkInterval < time.Second {
		checkInterval = time.Second
	}
	syncInterval := cfg.SyncInterval
	if syncInterval != 0 && syncInterval < time.Second {
		syncInterval = time.Second
	}
	m := &ChannelManager{
		endpoint:  endpoint.GetEndpoint(),
		dbManager: GetDBManager(),
		mcManager: mediaChannel.GetManager(),

		channels: make(map[string]*Channel),

		checkInterval: checkInterval,
		syncInterval:  syncInterval,
		syncSignal:    make(chan struct{}, 1),

		storageBufferSize: cfg.StorageBuffer.Uint(),
	}

	config.InitModuleLogger(m, ChannelManagerModule, ChannelManagerModuleName)
	config.RegisterLoggerConfigReloadedCallback(m, ChannelManagerModule, ChannelManagerModuleName)

	m.runner = lifecycle.NewWithInterruptedRun(m.start, m.run)
	m.Lifecycle = m.runner
	return m
}

func (m *ChannelManager) syncChannels() error {
	var devices []modelPkg.Device
	if res := m.dbManager.Table(modelPkg.DeviceTableName).Limit(MaxDevice).Find(&devices); res.Error != nil {
		return m.Logger().ErrorWith("加载设备信息失败", res.Error)
	} else {
		m.Logger().Info("加载设备信息成功", log.Int("设备数量", len(devices)))
	}

	var channels []modelPkg.DeviceChannel
	if res := m.dbManager.Table(modelPkg.DeviceChannelTableName).Where("status = ?", 1).Limit(MaxDeviceChannel).Find(&channels); res.Error != nil {
		return m.Logger().ErrorWith("加载在线设备通道信息失败", res.Error)
	} else {
		m.Logger().Info("加载在线设备通道信息成功", log.Int("在线设备通道数量", len(channels)))
	}

	var superviseTargets []modelPkg.SuperviseTarget
	if res := m.dbManager.Table(modelPkg.SuperviseTargetTableName).Limit(MaxSuperviseTarget).Find(&superviseTargets); res.Error != nil {
		return m.Logger().ErrorWith("加载监视物信息失败", res.Error)
	} else {
		m.Logger().Info("加载监视物信息成功", log.Int("监视物数量", len(superviseTargets)))
	}

	var superviseTargetTypes []modelPkg.SuperviseTargetType
	if res := m.dbManager.Table(modelPkg.SuperviseTargetTypeTableName).Limit(MaxSuperviseTargetType).Find(&superviseTargetTypes); res.Error != nil {
		return m.Logger().ErrorWith("加载监视物类型失败", res.Error)
	} else {
		m.Logger().Info("加载监视物类型成功", log.Int("监视物类型数量", len(superviseTargetTypes)))
	}

	var trains []modelPkg.Train
	var train *modelPkg.Train
	if res := m.dbManager.Table(modelPkg.TrainTableName).Limit(1).Find(&trains); res.Error != nil {
		return m.Logger().ErrorWith("加载车辆信息失败", res.Error)
	} else if len(trains) == 0 {
		m.Logger().Warn("未找到车辆信息")
	} else {
		m.Logger().Info("加载车辆信息成功", log.Int("车辆数量", len(trains)))
		train = &trains[0]
	}

	superviseTargetTypeMap := make(map[int32]*modelPkg.SuperviseTargetType)
	for i := range superviseTargetTypes {
		superviseTargetType := &superviseTargetTypes[i]
		if superviseTargetTypeMap[superviseTargetType.Type] != nil {
			m.Logger().Warn("重复的监视物类型", log.Int32("监视物类型ID", superviseTargetType.Type))
			continue
		}
		superviseTargetTypeMap[superviseTargetType.Type] = superviseTargetType
	}

	superviseTargetMap := make(map[int32]*modelPkg.SuperviseTarget)
	for i := range superviseTargets {
		superviseTarget := &superviseTargets[i]
		if superviseTargetTypeMap[superviseTarget.Type] == nil {
			m.Logger().Warn("未找到监视物对应的类型，忽略此监视物",
				log.String("监视物名称", superviseTarget.Name),
				log.Int32("监视物类型ID", superviseTarget.Type),
			)
			continue
		}
		if superviseTargetMap[superviseTarget.Id] != nil {
			m.Logger().Warn("重复的监视物信息", log.Int32("监视物ID", superviseTarget.Id))
			continue
		}
		superviseTargetMap[superviseTarget.Id] = superviseTarget
	}

	deviceMap := make(map[string]*modelPkg.Device)
	for i := range devices {
		device := &devices[i]
		if device.DeviceId == "" {
			m.Logger().Warn("设备ID为空，忽略此设备", log.Int32("设备主键ID", device.Id))
			continue
		} else if device.Ip == "" {
			m.Logger().Warn("设备IP为空，忽略此设备", log.String("设备ID", device.DeviceId))
			continue
		}
		if superviseTarget := superviseTargetMap[device.SuperviseTargetId]; superviseTarget != nil {
			superviseTargetType := superviseTargetTypeMap[device.SuperviseTargetType]
			if superviseTargetType == nil || superviseTargetType.Type != superviseTarget.Type {
				var superviseTargetTypeId int32
				if superviseTargetType != nil {
					superviseTargetTypeId = superviseTargetType.Type
				}
				m.Logger().Warn("设备监视物类型与设备监视物关联的类型不匹配",
					log.Int32("监视物类型ID", superviseTargetTypeId),
					log.Int32("监视物关联的类型ID", superviseTarget.Type),
				)
				continue
			}
		}
		if deviceMap[device.DeviceId] != nil {
			m.Logger().Warn("重复的设备ID", log.String("设备ID", device.DeviceId))
			continue
		}
		deviceMap[device.DeviceId] = device
	}

	channelMap := make(map[string]*modelPkg.DeviceChannel)
	for i := range channels {
		channel := &channels[i]
		if device := deviceMap[channel.DeviceId]; device != nil {
			id := device.DeviceId + "_" + channel.ChannelId
			if channelMap[id] != nil {
				m.Logger().Warn("重复的设备通道", log.String("设备ID", device.DeviceId), log.String("设备通道ID", channel.ChannelId))
				continue
			}
			channelMap[id] = channel
		} else {
			m.Logger().Warn("未找到设备通道对应的设备信息", log.String("设备ID", channel.ChannelId), log.String("设备通道ID", channel.DeviceId))
		}
	}

	oldChannels := mapUtils.Copy(m.channels)
	var removeChannels []*Channel
	for id, model := range channelMap {
		if channel := oldChannels[id]; channel != nil {
			delete(oldChannels, id)
			mc, _ := m.mcManager.Create(id, mediaChannel.WithStorageType("raw"))
			if mc != channel.mc {
				m.Logger().Warn("媒体通道被删除或更换")
				removeChannels = append(removeChannels, channel)
				delete(m.channels, id)
				continue
			}
		} else {
			mc, err := m.mcManager.Create(id, mediaChannel.WithStorageType("raw"))
			if err != nil {
				if _, is := err.(*errors.Exist); is {
					m.mcManager.Logger().Warn("媒体通道已经存在", log.String("ID", id))
				}
				continue
			}
			m.channels[id] = &Channel{
				mc:    mc,
				model: model,
			}
			m.Logger().Info("添加设备通道", log.String("设备ID", model.DeviceId), log.String("通道ID", model.ChannelId))
		}
	}
	for id, channel := range oldChannels {
		removeChannels = append(removeChannels, channel)
		delete(m.channels, id)
	}

	waiter := syncPkg.WaitGroup{}
	for _, channel := range m.channels {
		mc, model := channel.mc, channel.model
		device := deviceMap[model.DeviceId]
		mc.SetField("device", device.Clone())
		mc.SetField("channel", model.Clone())
		mc.SetField("supervise-target", superviseTargetMap[device.SuperviseTargetId].Clone())
		mc.SetField("supervise-target-type", superviseTargetTypeMap[device.SuperviseTargetType].Clone())
		mc.SetField("train", train.Clone())
		m.checkChannel(channel, &waiter)
	}
	waiter.Wait()

	m.closeChannels(removeChannels)
	utils.ChanTryPop(m.syncSignal)

	return nil
}

func (m *ChannelManager) checkChannel(channel *Channel, waiter *syncPkg.WaitGroup) (err error) {
	mc, model := channel.mc, channel.model
	localIp := config.MediaRTPConfig().GetLocalIP()
	if err := mc.SetupStorage(time.Hour*24, m.storageBufferSize); err != nil {
		return err
	}
	sc := mc.StorageChannel()
	if fsc, is := sc.(*fileStorage.Channel); is {
		fsc.SetFileNameFormatter(func(c storage.FileChannel, seq uint64, createTime time.Time, storageType uint32) string {
			suffix := ".mpg"
			trainNo := "未知车次"
			carriageNo := "未知车厢"
			position := "未知位置"
			if field := c.Field("train"); field != nil {
				if train := field.(*modelPkg.Train); train != nil {
					if train.TrainNo != "" {
						trainNo = train.TrainNo
					}
				}
			}
			if field := c.Field("device"); field != nil {
				if device := field.(*modelPkg.Device); device != nil {
					if device.CarriageNo != 0 {
						carriageNo = fmt.Sprintf("车厢%d", device.CarriageNo)
					}
					if device.Position != "" {
						position = device.Position
					}
				}
			}
			return fmt.Sprintf("%s-%s-%s-%s-%d%s", trainNo, carriageNo, position, createTime.Format("20060102150405"), seq, suffix)
		})
	}
	if err := mc.StartRecord(); err != nil {
		return err
	}
	player, err := mc.OpenRtpPlayer(nil)
	if err != nil {
		if _, is := err.(*errors.Exist); is {
			return nil
		}
		return err
	}
	defer func() {
		if err != nil {
			if waiter, err := mc.CloseRtpPlayer(); err == nil {
				<-waiter
			}
		}
	}()
	streamPlayer, err := player.AddStream("tcp")
	if err != nil {
		return err
	}
	waiter.Add(1)
	go func() (err error) {
		defer func() {
			if err != nil {
				if waiter, err := mc.CloseRtpPlayer(); err == nil {
					<-waiter
				}
			}
			waiter.Done()
		}()
		mediaInfo, err := m.endpoint.StartStream(model.ChannelId, model.DeviceId, "tcp", localIp, streamPlayer.LocalPort())
		if err != nil {
			return err
		}
		var streamInfo *media.RtpStreamInfo
		for _, sdpRtpMap := range mediaInfo.RtpMap {
			mediaType := media.ParseMediaType(sdpRtpMap.Format)
			if mediaType == nil {
				continue
			}
			if sdpRtpMap.Type >= 128 || sdpRtpMap.Type < 0 {
				continue
			}
			streamInfo = media.NewRtpStreamInfo(mediaType, nil, uint8(sdpRtpMap.Type), sdpRtpMap.Rate)
		}
		if streamInfo == nil {
			err = errors.NewNotFound("RTP流信息")
			m.Logger().Error(err.Error())
			return err
		}
		err = streamPlayer.Setup(mediaInfo.RemoteIP, mediaInfo.RemotePort, mediaInfo.SSRC, streamInfo, mediaChannel.SetupConfig{})
		return err
	}()
	return nil
}

func (m *ChannelManager) closeChannels(channels []*Channel) {
	var closedWaiters []<-chan error
	for _, channel := range channels {
		mc, model := channel.mc, channel.model
		m.endpoint.StopStream(model.ChannelId, model.DeviceId)
		mc.Close(nil)
		closedWaiters = append(closedWaiters, mc.ClosedWaiter())
		m.Logger().Info("删除设备通道", log.String("设备ID", model.DeviceId), log.String("通道ID", model.ChannelId))
	}
	for _, closedWaiter := range closedWaiters {
		<-closedWaiter
	}
}

func (m *ChannelManager) checkChannels() {
	waiter := syncPkg.WaitGroup{}
	for _, channel := range m.channels {
		m.checkChannel(channel, &waiter)
	}
	waiter.Wait()
}

func (m *ChannelManager) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	return m.syncChannels()
}

func (m *ChannelManager) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	checkOrSyncSignal := make(chan struct{}, 1)
	timer := timerPkg.NewTimer(checkOrSyncSignal)
	defer timer.Stop()

	var interval time.Duration
	if m.syncInterval != 0 {
		interval = m.syncInterval
	} else {
		interval = m.checkInterval
	}
	if interval > 0 {
		timer.After(interval)
	}
	for {
		select {
		case <-checkOrSyncSignal:
			if m.syncInterval != 0 {
				m.syncChannels()
			} else {
				m.checkChannels()
			}
			timer.After(interval)
		case <-m.syncSignal:
			m.syncChannels()
			timer.After(interval)
			utils.ChanTryPop(checkOrSyncSignal)
		case <-interrupter:
			m.closeChannels(mapUtils.Values(m.channels))
			return nil
		}
	}
}

func (m *ChannelManager) Sync() {
	if !utils.ChanTryPush(m.syncSignal, struct{}{}) {
		m.Logger().Warn("同步信号处理繁忙")
	}
}

var GetChannelManager = component.NewPointer(NewChannelManager).Get
