package config

import (
	"gitee.com/sy_183/common/config"
	"gitee.com/sy_183/common/log"
	logConfig "gitee.com/sy_183/common/log/config"
	"gitee.com/sy_183/common/sgr"
	"gitee.com/sy_183/common/uns"
)

const (
	Module     = "config"
	ModuleName = "配置管理器"
)

var (
	defaultLogger = config.MustHandleWith(new(logConfig.LoggerConfig)).MustBuild()
	logger        log.AtomicLogger
)

func init() {
	InitModuleDefaultLogger(&logger, ModuleName)
	RegisterLoggerConfigReloadedCallback(&logger, Module, ModuleName)
}

func InitModuleDefaultLogger(logger log.LoggerProvider, moduleName string) {
	logger.SetLogger(defaultLogger.Named(moduleName))
}

func logConfigError(logger *log.Logger, moduleName string, err error) {
	logger.Warn("日志配置失败", log.String("模块", moduleName), log.NamedError("错误", err))
}

func logDefaultConfigError(logger *log.Logger, err error) {
	logger.Warn("日志默认配置失败", log.NamedError("错误", err))
}

func eastAsianWidth(ch rune) string {
	if (0x3000 == ch) ||
		(0xFF01 <= ch && ch <= 0xFF60) ||
		(0xFFE0 <= ch && ch <= 0xFFE6) {
		return "F"
	}
	if (0x20A9 == ch) ||
		(0xFF61 <= ch && ch <= 0xFFBE) ||
		(0xFFC2 <= ch && ch <= 0xFFC7) ||
		(0xFFCA <= ch && ch <= 0xFFCF) ||
		(0xFFD2 <= ch && ch <= 0xFFD7) ||
		(0xFFDA <= ch && ch <= 0xFFDC) ||
		(0xFFE8 <= ch && ch <= 0xFFEE) {
		return "H"
	}
	if (0x1100 <= ch && ch <= 0x115F) ||
		(0x11A3 <= ch && ch <= 0x11A7) ||
		(0x11FA <= ch && ch <= 0x11FF) ||
		(0x2329 <= ch && ch <= 0x232A) ||
		(0x2E80 <= ch && ch <= 0x2E99) ||
		(0x2E9B <= ch && ch <= 0x2EF3) ||
		(0x2F00 <= ch && ch <= 0x2FD5) ||
		(0x2FF0 <= ch && ch <= 0x2FFB) ||
		(0x3001 <= ch && ch <= 0x303E) ||
		(0x3041 <= ch && ch <= 0x3096) ||
		(0x3099 <= ch && ch <= 0x30FF) ||
		(0x3105 <= ch && ch <= 0x312D) ||
		(0x3131 <= ch && ch <= 0x318E) ||
		(0x3190 <= ch && ch <= 0x31BA) ||
		(0x31C0 <= ch && ch <= 0x31E3) ||
		(0x31F0 <= ch && ch <= 0x321E) ||
		(0x3220 <= ch && ch <= 0x3247) ||
		(0x3250 <= ch && ch <= 0x32FE) ||
		(0x3300 <= ch && ch <= 0x4DBF) ||
		(0x4E00 <= ch && ch <= 0xA48C) ||
		(0xA490 <= ch && ch <= 0xA4C6) ||
		(0xA960 <= ch && ch <= 0xA97C) ||
		(0xAC00 <= ch && ch <= 0xD7A3) ||
		(0xD7B0 <= ch && ch <= 0xD7C6) ||
		(0xD7CB <= ch && ch <= 0xD7FB) ||
		(0xF900 <= ch && ch <= 0xFAFF) ||
		(0xFE10 <= ch && ch <= 0xFE19) ||
		(0xFE30 <= ch && ch <= 0xFE52) ||
		(0xFE54 <= ch && ch <= 0xFE66) ||
		(0xFE68 <= ch && ch <= 0xFE6B) ||
		(0x1B000 <= ch && ch <= 0x1B001) ||
		(0x1F200 <= ch && ch <= 0x1F202) ||
		(0x1F210 <= ch && ch <= 0x1F23A) ||
		(0x1F240 <= ch && ch <= 0x1F248) ||
		(0x1F250 <= ch && ch <= 0x1F251) ||
		(0x20000 <= ch && ch <= 0x2F73F) ||
		(0x2B740 <= ch && ch <= 0x2FFFD) ||
		(0x30000 <= ch && ch <= 0x3FFFD) {
		return "W"
	}
	if (0x0020 <= ch && ch <= 0x007E) ||
		(0x00A2 <= ch && ch <= 0x00A3) ||
		(0x00A5 <= ch && ch <= 0x00A6) ||
		(0x00AC == ch) ||
		(0x00AF == ch) ||
		(0x27E6 <= ch && ch <= 0x27ED) ||
		(0x2985 <= ch && ch <= 0x2986) {
		return "Na"
	}
	if (0x00A1 == ch) ||
		(0x00A4 == ch) ||
		(0x00A7 <= ch && ch <= 0x00A8) ||
		(0x00AA == ch) ||
		(0x00AD <= ch && ch <= 0x00AE) ||
		(0x00B0 <= ch && ch <= 0x00B4) ||
		(0x00B6 <= ch && ch <= 0x00BA) ||
		(0x00BC <= ch && ch <= 0x00BF) ||
		(0x00C6 == ch) ||
		(0x00D0 == ch) ||
		(0x00D7 <= ch && ch <= 0x00D8) ||
		(0x00DE <= ch && ch <= 0x00E1) ||
		(0x00E6 == ch) ||
		(0x00E8 <= ch && ch <= 0x00EA) ||
		(0x00EC <= ch && ch <= 0x00ED) ||
		(0x00F0 == ch) ||
		(0x00F2 <= ch && ch <= 0x00F3) ||
		(0x00F7 <= ch && ch <= 0x00FA) ||
		(0x00FC == ch) ||
		(0x00FE == ch) ||
		(0x0101 == ch) ||
		(0x0111 == ch) ||
		(0x0113 == ch) ||
		(0x011B == ch) ||
		(0x0126 <= ch && ch <= 0x0127) ||
		(0x012B == ch) ||
		(0x0131 <= ch && ch <= 0x0133) ||
		(0x0138 == ch) ||
		(0x013F <= ch && ch <= 0x0142) ||
		(0x0144 == ch) ||
		(0x0148 <= ch && ch <= 0x014B) ||
		(0x014D == ch) ||
		(0x0152 <= ch && ch <= 0x0153) ||
		(0x0166 <= ch && ch <= 0x0167) ||
		(0x016B == ch) ||
		(0x01CE == ch) ||
		(0x01D0 == ch) ||
		(0x01D2 == ch) ||
		(0x01D4 == ch) ||
		(0x01D6 == ch) ||
		(0x01D8 == ch) ||
		(0x01DA == ch) ||
		(0x01DC == ch) ||
		(0x0251 == ch) ||
		(0x0261 == ch) ||
		(0x02C4 == ch) ||
		(0x02C7 == ch) ||
		(0x02C9 <= ch && ch <= 0x02CB) ||
		(0x02CD == ch) ||
		(0x02D0 == ch) ||
		(0x02D8 <= ch && ch <= 0x02DB) ||
		(0x02DD == ch) ||
		(0x02DF == ch) ||
		(0x0300 <= ch && ch <= 0x036F) ||
		(0x0391 <= ch && ch <= 0x03A1) ||
		(0x03A3 <= ch && ch <= 0x03A9) ||
		(0x03B1 <= ch && ch <= 0x03C1) ||
		(0x03C3 <= ch && ch <= 0x03C9) ||
		(0x0401 == ch) ||
		(0x0410 <= ch && ch <= 0x044F) ||
		(0x0451 == ch) ||
		(0x2010 == ch) ||
		(0x2013 <= ch && ch <= 0x2016) ||
		(0x2018 <= ch && ch <= 0x2019) ||
		(0x201C <= ch && ch <= 0x201D) ||
		(0x2020 <= ch && ch <= 0x2022) ||
		(0x2024 <= ch && ch <= 0x2027) ||
		(0x2030 == ch) ||
		(0x2032 <= ch && ch <= 0x2033) ||
		(0x2035 == ch) ||
		(0x203B == ch) ||
		(0x203E == ch) ||
		(0x2074 == ch) ||
		(0x207F == ch) ||
		(0x2081 <= ch && ch <= 0x2084) ||
		(0x20AC == ch) ||
		(0x2103 == ch) ||
		(0x2105 == ch) ||
		(0x2109 == ch) ||
		(0x2113 == ch) ||
		(0x2116 == ch) ||
		(0x2121 <= ch && ch <= 0x2122) ||
		(0x2126 == ch) ||
		(0x212B == ch) ||
		(0x2153 <= ch && ch <= 0x2154) ||
		(0x215B <= ch && ch <= 0x215E) ||
		(0x2160 <= ch && ch <= 0x216B) ||
		(0x2170 <= ch && ch <= 0x2179) ||
		(0x2189 == ch) ||
		(0x2190 <= ch && ch <= 0x2199) ||
		(0x21B8 <= ch && ch <= 0x21B9) ||
		(0x21D2 == ch) ||
		(0x21D4 == ch) ||
		(0x21E7 == ch) ||
		(0x2200 == ch) ||
		(0x2202 <= ch && ch <= 0x2203) ||
		(0x2207 <= ch && ch <= 0x2208) ||
		(0x220B == ch) ||
		(0x220F == ch) ||
		(0x2211 == ch) ||
		(0x2215 == ch) ||
		(0x221A == ch) ||
		(0x221D <= ch && ch <= 0x2220) ||
		(0x2223 == ch) ||
		(0x2225 == ch) ||
		(0x2227 <= ch && ch <= 0x222C) ||
		(0x222E == ch) ||
		(0x2234 <= ch && ch <= 0x2237) ||
		(0x223C <= ch && ch <= 0x223D) ||
		(0x2248 == ch) ||
		(0x224C == ch) ||
		(0x2252 == ch) ||
		(0x2260 <= ch && ch <= 0x2261) ||
		(0x2264 <= ch && ch <= 0x2267) ||
		(0x226A <= ch && ch <= 0x226B) ||
		(0x226E <= ch && ch <= 0x226F) ||
		(0x2282 <= ch && ch <= 0x2283) ||
		(0x2286 <= ch && ch <= 0x2287) ||
		(0x2295 == ch) ||
		(0x2299 == ch) ||
		(0x22A5 == ch) ||
		(0x22BF == ch) ||
		(0x2312 == ch) ||
		(0x2460 <= ch && ch <= 0x24E9) ||
		(0x24EB <= ch && ch <= 0x254B) ||
		(0x2550 <= ch && ch <= 0x2573) ||
		(0x2580 <= ch && ch <= 0x258F) ||
		(0x2592 <= ch && ch <= 0x2595) ||
		(0x25A0 <= ch && ch <= 0x25A1) ||
		(0x25A3 <= ch && ch <= 0x25A9) ||
		(0x25B2 <= ch && ch <= 0x25B3) ||
		(0x25B6 <= ch && ch <= 0x25B7) ||
		(0x25BC <= ch && ch <= 0x25BD) ||
		(0x25C0 <= ch && ch <= 0x25C1) ||
		(0x25C6 <= ch && ch <= 0x25C8) ||
		(0x25CB == ch) ||
		(0x25CE <= ch && ch <= 0x25D1) ||
		(0x25E2 <= ch && ch <= 0x25E5) ||
		(0x25EF == ch) ||
		(0x2605 <= ch && ch <= 0x2606) ||
		(0x2609 == ch) ||
		(0x260E <= ch && ch <= 0x260F) ||
		(0x2614 <= ch && ch <= 0x2615) ||
		(0x261C == ch) ||
		(0x261E == ch) ||
		(0x2640 == ch) ||
		(0x2642 == ch) ||
		(0x2660 <= ch && ch <= 0x2661) ||
		(0x2663 <= ch && ch <= 0x2665) ||
		(0x2667 <= ch && ch <= 0x266A) ||
		(0x266C <= ch && ch <= 0x266D) ||
		(0x266F == ch) ||
		(0x269E <= ch && ch <= 0x269F) ||
		(0x26BE <= ch && ch <= 0x26BF) ||
		(0x26C4 <= ch && ch <= 0x26CD) ||
		(0x26CF <= ch && ch <= 0x26E1) ||
		(0x26E3 == ch) ||
		(0x26E8 <= ch && ch <= 0x26FF) ||
		(0x273D == ch) ||
		(0x2757 == ch) ||
		(0x2776 <= ch && ch <= 0x277F) ||
		(0x2B55 <= ch && ch <= 0x2B59) ||
		(0x3248 <= ch && ch <= 0x324F) ||
		(0xE000 <= ch && ch <= 0xF8FF) ||
		(0xFE00 <= ch && ch <= 0xFE0F) ||
		(0xFFFD == ch) ||
		(0x1F100 <= ch && ch <= 0x1F10A) ||
		(0x1F110 <= ch && ch <= 0x1F12D) ||
		(0x1F130 <= ch && ch <= 0x1F169) ||
		(0x1F170 <= ch && ch <= 0x1F19A) ||
		(0xE0100 <= ch && ch <= 0xE01EF) ||
		(0xF0000 <= ch && ch <= 0xFFFFD) ||
		(0x100000 <= ch && ch <= 0x10FFFD) {
		return "A"
	}

	return "N"
}

func charWidth(ch rune) int {
	switch eaw := eastAsianWidth(ch); eaw {
	case "F", "W", "A":
		return 2
	default:
		return 1
	}
}

func ReloadModuleLogger(oc, nc *Config, logger log.LoggerProvider, module, moduleName string) {
	var width int
	for _, ch := range moduleName {
		width += charWidth(ch)
	}
	if width < 36 {
		fill := make([]byte, 36-width)
		for i := range fill {
			fill[i] = ' '
		}
		moduleName += uns.BytesToString(fill)
	}
	moduleName = sgr.WrapColor(moduleName, sgr.FgCyan)
	if nl, err := nc.Log.Build(module); err == nil {
		// 使用模块指定的日志配置
		logger.SetLogger(nl.Named(moduleName))
	} else {
		// 模块指定的日志配置失败，如果是第一次初始化，则使用配置文件中默认的日志配置，否则不更改日志配置
		if oc == nil {
			if nl, err := nc.Log.Build(module); err == nil {
				logger.SetLogger(nl.Named(moduleName))
				logConfigError(logger.Logger(), moduleName, err)
			} else {
				// 如果配置文件中默认的日志配置也失败，则使用默认的日志配置
				logger.CompareAndSwapLogger(nil, defaultLogger.Named(moduleName))
				logConfigError(logger.Logger(), moduleName, err)
				logDefaultConfigError(logger.Logger(), err)
			}
		} else {
			logConfigError(logger.Logger(), moduleName, err)
		}
	}
}

func InitModuleLogger(logger log.LoggerProvider, module, moduleName string) {
	ReloadModuleLogger(nil, GlobalConfig(), logger, module, moduleName)
}

func RegisterLoggerConfigReloadedCallback(logger log.LoggerProvider, module, moduleName string) uint64 {
	return Context().RegisterConfigReloadedCallback(LoggerConfigReloadedCallback(logger, module, moduleName))
}

func UnregisterLoggerConfigReloadedCallback(id uint64) {
	Context().UnregisterConfigReloadedCallback(id)
}

func LoggerConfigReloadedCallback(logger log.LoggerProvider, module, moduleName string) config.OnConfigReloaded[Config] {
	return func(oc, nc *Config) { ReloadModuleLogger(oc, nc, logger, module, moduleName) }
}

func Logger() *log.Logger {
	return logger.Logger()
}

func DefaultLogger() *log.Logger {
	return defaultLogger
}
