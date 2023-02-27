package config

import (
	"gitee.com/sy_183/common/component"
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/log"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	cobra.MousetrapHelpText = ""
}

type ArgsConfig struct {
	Help       bool
	ConfigFile string
	Command    string
}

var args = component.Pointer[ArgsConfig]{Init: func() *ArgsConfig {
	a := new(ArgsConfig)
	config.Handle(a)
	command := cobra.Command{
		Use:   "cvdsmas",
		Short: "CVDS媒体接入服务",
		Long:  "CVDS媒体接入服务",
		Run: func(cmd *cobra.Command, args []string) {
			a.Help = false
			if len(args) > 0 {
				a.Command = args[0]
			}
		},
	}
	command.Flags().StringVarP(&a.ConfigFile, "config", "f", "", "指定程序配置文件")
	command.Flags().BoolVarP(&a.Help, "help", "h", false, "显示帮助信息")
	if err := command.Execute(); err != nil {
		Logger().Fatal("解析命令行参数失败", log.Error(err))
	}
	if a.Help {
		os.Exit(0)
	}
	return a
}}

func GetArgs() *ArgsConfig {
	return args.Get()
}
