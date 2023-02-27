package app

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/cvds-mas/api"
	"gitee.com/sy_183/cvds-mas/config"
	"gitee.com/sy_183/cvds-mas/db"
	"gitee.com/sy_183/cvds-mas/gb28181"
	mediaChannel "gitee.com/sy_183/cvds-mas/media/channel"
	"gitee.com/sy_183/cvds-mas/media/rtp"
	rtpServer "gitee.com/sy_183/rtp/server"
)

type App struct {
	lifecycle.Lifecycle
	list *lifecycle.List

	gb28181DBManager            *db.DBManager
	gb28181ChannelManager       *gb28181.ChannelManager
	gb28181StorageConfigManager *gb28181.StorageConfigManager

	apiServer           *api.Server
	mediaChannelManager *mediaChannel.Manager
	rtpTCPServer        *lifecycle.Retryable[*rtpServer.TCPServer]
	rtpManager          *rtpServer.Manager
}

func newApp() *App {
	app := &App{
		gb28181DBManager:            gb28181.GetDBManager(),
		gb28181ChannelManager:       gb28181.GetChannelManager(),
		gb28181StorageConfigManager: gb28181.GetStorageConfigManager(),

		apiServer:           api.GetServer(),
		mediaChannelManager: mediaChannel.GetManager(),
		rtpTCPServer:        rtp.GetRetryableTCPServer(),
		rtpManager:          rtp.GetManager(),
	}

	app.list = lifecycle.NewList().
		MustAppend(lifecycle.NewGroup().
			MustAdd(gb28181.DBManagerModule, app.gb28181DBManager).SetCloseAllOnExit(false).Group().
			MustAdd("media.rtp", lifecycle.NewList().
				MustAppend(lifecycle.NewGroup().
					MustAdd(rtp.ManagerModule, app.rtpManager).Group().
					MustAdd(rtp.TCPServerModule, app.rtpTCPServer).Group(),
				).List().
				MustAppend(app.mediaChannelManager).List(),
			).Group(),
		).List().
		MustAppend(lifecycle.NewGroup().
			MustAdd(gb28181.ChannelManagerModule, app.gb28181ChannelManager).Group().
			MustAdd(gb28181.StorageConfigManagerModule, app.gb28181StorageConfigManager).Group(),
		).List().
		MustAppend(app.apiServer).List()

	app.Lifecycle = lifecycle.NewWithInterruptedRun(app.start, app.run)
	return app
}

func (a *App) handleStopSignal() error {
	a.list.Close(nil)
	return <-a.list.ClosedWaiter()
}

func (a *App) start(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	a.list.Background()
	for {
		select {
		case err := <-a.list.StartedWaiter():
			if err != nil {
				return err
			}
			return nil
		case <-interrupter:
			a.list.Close(nil)
		}
	}
}

func (a *App) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	sys := api.GetSys()
	closedWaiter := a.list.ClosedWaiter()
	for {
		select {
		case err := <-closedWaiter:
			return err

		case <-sys.ReloadConfigSignal:
			config.Context().ReloadConfig()

		case <-sys.RestartSignal:
			// 关闭所有服务
			a.list.Close(nil)
			if err := <-a.list.ClosedWaiter(); err != nil {
				return err
			}
			select {
			case <-sys.StopSignal:
				return nil
			case <-interrupter:
				return nil
			default:
			}

			// 重新加载配置文件
			config.Context().ReloadConfig()

			// 启动所有组件
			a.list.Background()
			select {
			case err := <-a.list.StartedWaiter():
				if err != nil {
					return err
				}
			case <-sys.StopSignal:
				return a.handleStopSignal()
			case <-interrupter:
				return a.handleStopSignal()
			}

		case <-sys.StopSignal:
			return a.handleStopSignal()
		case <-interrupter:
			return a.handleStopSignal()
		}
	}
}

var GetApp = component.NewPointer(newApp).Get
