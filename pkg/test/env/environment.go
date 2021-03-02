package env

import (
	"fmt"
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/clock"
	"github.com/applike/gosoline/pkg/mon"
	"github.com/stretchr/testify/assert"
	"testing"
)

type Environment struct {
	componentOptions []ComponentOption
	configOptions    []ConfigOption
	loggerOptions    []LoggerOption

	t          *testing.T
	config     cfg.GosoConf
	logger     mon.GosoLog
	filesystem *filesystem
	runner     *containerRunner
	components *ComponentsContainer
}

func NewEnvironment(t *testing.T, options ...Option) (*Environment, error) {
	env := &Environment{
		t: t,
	}

	for _, opt := range options {
		opt(env)
	}

	config := cfg.New()
	for _, opt := range env.configOptions {
		if err := opt(config); err != nil {
			return nil, fmt.Errorf("can apply config option: %w", err)
		}
	}

	logger := mon.NewLogger()
	for _, opt := range env.loggerOptions {
		if err := opt(config, logger); err != nil {
			return nil, fmt.Errorf("can apply logger option: %w", err)
		}
	}

	if err := cfg.ApplyPostProcessors(config, logger); err != nil {
		return nil, fmt.Errorf("can not apply post processor on config: %w", err)
	}

	env.config = config
	env.logger = logger
	env.filesystem = newFilesystem(t)
	env.runner = NewContainerRunner(config, logger)

	var err error
	var skeletons []*componentSkeleton
	var component Component
	var containers map[string]*container
	var components = NewComponentsContainer()
	var componentConfigManger = NewComponentsConfigManager(config)

	for _, opt := range env.componentOptions {
		if err := opt(componentConfigManger); err != nil {
			return nil, fmt.Errorf("can apply component option: %w", err)
		}
	}

	for typ, factory := range componentFactories {
		if err = factory.Detect(config, componentConfigManger); err != nil {
			return env, fmt.Errorf("can not autodetect components for %s: %w", typ, err)
		}
	}

	if skeletons, err = buildComponentSkeletons(componentConfigManger); err != nil {
		return env, fmt.Errorf("can not create component skeletons: %w", err)
	}

	if containers, err = env.runner.RunContainers(skeletons); err != nil {
		return env, err
	}

	for _, skeleton := range skeletons {
		container := containers[skeleton.id()]

		if component, err = buildComponent(config, logger, skeleton, container); err != nil {
			return env, fmt.Errorf("can not build component %s: %w", skeleton.id(), err)
		}

		component.SetT(t)
		components.Add(skeleton.typ, skeleton.name, component)
	}

	if err = config.Option(components.GetCfgOptions()...); err != nil {
		return nil, fmt.Errorf("can not apply cfg options from components: %w", err)
	}

	env.components = components

	return env, nil
}

func (e *Environment) addComponentOption(opt ComponentOption) {
	e.componentOptions = append(e.componentOptions, opt)
}

func (e *Environment) addConfigOption(opt ConfigOption) {
	e.configOptions = append(e.configOptions, opt)
}

func (e *Environment) addLoggerOption(opt LoggerOption) {
	e.loggerOptions = append(e.loggerOptions, opt)
}

func (e *Environment) Stop() error {
	return e.runner.Stop()
}

func (e *Environment) Config() cfg.GosoConf {
	return e.config
}

func (e *Environment) Logger() mon.GosoLog {
	return e.logger
}

func (e *Environment) Clock() clock.Clock {
	return clock.Provider
}

func (e *Environment) Filesystem() *filesystem {
	return e.filesystem
}

func (e *Environment) Component(typ string, name string) Component {
	var err error
	var component Component

	if component, err = e.components.Get(typ, name); err != nil {
		assert.FailNow(e.t, "can not get component", err.Error())
	}

	return component
}

func (e *Environment) DynamoDb(name string) *DdbComponent {
	return e.Component(componentDdb, name).(*DdbComponent)
}

func (e *Environment) Localstack(name string) *localstackComponent {
	return e.Component(ComponentLocalstack, name).(*localstackComponent)
}

func (e *Environment) MySql(name string) *mysqlComponent {
	return e.Component(componentMySql, name).(*mysqlComponent)
}

func (e *Environment) Wiremock(name string) *wiremockComponent {
	return e.Component(componentWiremock, name).(*wiremockComponent)
}

func (e *Environment) StreamInput(name string) *streamInputComponent {
	return e.Component(componentStreamInput, name).(*streamInputComponent)
}

func (e *Environment) StreamOutput(name string) *streamOutputComponent {
	return e.Component(componentStreamOutput, name).(*streamOutputComponent)
}

func (e *Environment) Toxiproxy(name string) *ToxiproxyComponent {
	return e.Component(componentToxiproxy, name).(*ToxiproxyComponent)
}
