package suite

import (
	"fmt"
	"github.com/applike/gosoline/pkg/clock"
	"github.com/applike/gosoline/pkg/ipread"
	"github.com/applike/gosoline/pkg/test/env"
	"github.com/spf13/cast"
	"time"
)

type suiteOptions struct {
	envOptions []env.Option
	envSetup   []func() error
}

func (s *suiteOptions) addEnvOption(opt env.Option) {
	s.envOptions = append(s.envOptions, opt)
}

type SuiteOption func(s *suiteOptions)

func WithClockProvider(clk clock.Clock) SuiteOption {
	return func(s *suiteOptions) {
		s.envSetup = append(s.envSetup, func() error {
			clock.Provider = clk
			return nil
		})
	}
}

func WithClockProviderAt(datetime string) SuiteOption {
	return func(s *suiteOptions) {
		s.envSetup = append(s.envSetup, func() error {
			var err error
			var dt time.Time

			if dt, err = cast.ToTimeE(datetime); err != nil {
				return fmt.Errorf("can not cast provided fake clock datetime %s: %w", datetime, err)
			}

			clock.Provider = clock.NewFakeClockAt(dt)

			return nil
		})
	}
}

func WithComponent(settings env.ComponentBaseSettingsAware) SuiteOption {
	return func(s *suiteOptions) {
		s.addEnvOption(env.WithComponent(settings))
	}
}

func WithConfigFile(file string) SuiteOption {
	return func(s *suiteOptions) {
		s.addEnvOption(env.WithConfigFile(file))
	}
}

func WithConfigMap(settings map[string]interface{}) SuiteOption {
	return func(s *suiteOptions) {
		s.addEnvOption(env.WithConfigMap(settings))
	}
}

func WithContainerExpireAfter(expireAfter time.Duration) SuiteOption {
	return func(s *suiteOptions) {
		s.addEnvOption(env.WithContainerExpireAfter(expireAfter))
	}
}

func WithEnvSetup(setups ...func() error) SuiteOption {
	return func(s *suiteOptions) {
		s.envSetup = append(s.envSetup, setups...)
	}
}

func WithLogLevel(level string) SuiteOption {
	return func(s *suiteOptions) {
		s.addEnvOption(env.WithLoggerLevel(level))
	}
}

func WithIpReadFromMemory(name string, records map[string]ipread.MemoryRecord) SuiteOption {
	provider := ipread.ProvideMemoryProvider(name)

	for ip, record := range records {
		provider.AddRecord(ip, record.CountryIso, record.CityName)
	}

	return func(s *suiteOptions) {
		key := fmt.Sprintf("ipread.%s.provider", name)
		s.addEnvOption(env.WithConfigSetting(key, "memory"))
	}
}

func WithoutAutoDetectedComponents(components ...string) SuiteOption {
	return func(s *suiteOptions) {
		s.addEnvOption(env.WithoutAutoDetectedComponents(components...))
	}
}
