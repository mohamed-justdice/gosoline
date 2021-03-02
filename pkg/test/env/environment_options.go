package env

import (
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/mon"
	"strings"
	"time"
)

type Option func(env *Environment)
type ComponentOption func(componentConfigManger *ComponentsConfigManager) error
type ConfigOption func(config cfg.GosoConf) error
type LoggerOption func(config cfg.GosoConf, logger mon.GosoLog) error

type loggerSettings struct {
	Level           string `cfg:"level" default:"info" validate:"required"`
	Format          string `cfg:"format" default:"console" validate:"required"`
	TimestampFormat string `cfg:"timestamp_format" default:"15:04:05.000" validate:"required"`
}

func WithComponent(settings ComponentBaseSettingsAware) Option {
	return func(env *Environment) {
		env.addComponentOption(func(componentConfigManger *ComponentsConfigManager) error {
			return componentConfigManger.Add(settings)
		})
	}
}

func WithConfigFile(file string) Option {
	return func(env *Environment) {
		env.addConfigOption(func(config cfg.GosoConf) error {
			return config.Option(cfg.WithConfigFile(file, "yml"))
		})
	}
}

func WithConfigEnvKeyReplacer(replacer *strings.Replacer) Option {
	return func(env *Environment) {
		env.addConfigOption(func(config cfg.GosoConf) error {
			return config.Option(cfg.WithEnvKeyReplacer(replacer))
		})
	}
}

func WithConfigMap(settings map[string]interface{}) Option {
	return func(env *Environment) {
		env.addConfigOption(func(config cfg.GosoConf) error {
			return config.Option(cfg.WithConfigMap(settings))
		})
	}
}

func WithConfigSetting(key string, settings interface{}) Option {
	return func(env *Environment) {
		env.addConfigOption(func(config cfg.GosoConf) error {
			return config.Option(cfg.WithConfigSetting(key, settings))
		})
	}
}

func WithContainerExpireAfter(expireAfter time.Duration) Option {
	return func(env *Environment) {
		env.addConfigOption(func(config cfg.GosoConf) error {
			return config.Option(cfg.WithConfigSetting("test.container_runner.expire_after", expireAfter.String()))
		})
	}
}

func WithLoggerLevel(level string) Option {
	return func(env *Environment) {
		env.addLoggerOption(func(_ cfg.GosoConf, logger mon.GosoLog) error {
			return logger.Option(mon.WithLevel(level))
		})
	}
}

func WithLoggerSettingsFromConfig(env *Environment) {
	env.addLoggerOption(func(config cfg.GosoConf, logger mon.GosoLog) error {
		settings := &loggerSettings{}
		config.UnmarshalKey("test.logger", settings)

		loggerOptions := []mon.LoggerOption{
			mon.WithLevel(settings.Level),
			mon.WithFormat(settings.Format),
			mon.WithTimestampFormat(settings.TimestampFormat),
		}

		return logger.Option(loggerOptions...)
	})
}

func WithoutAutoDetectedComponents(components ...string) Option {
	return func(env *Environment) {
		env.addConfigOption(func(config cfg.GosoConf) error {
			return config.Option(cfg.WithConfigSetting("test.auto_detect.skip_components", components))
		})
	}
}
