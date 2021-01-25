package env

import (
	"fmt"
	toxiproxy "github.com/Shopify/toxiproxy/client"
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/mon"
)

func init() {
	componentFactories[componentToxiproxy] = new(toxiproxyFactory)
}

const componentToxiproxy = "toxiproxy"

type ToxiproxySettings struct {
	ComponentBaseSettings
	ComponentContainerSettings
	Port int `cfg:"port" default:"0"`
}

type toxiproxyFactory struct {
}

func (f *toxiproxyFactory) Detect(config cfg.Config, manager *ComponentsConfigManager) error {
	return nil
}

func (f *toxiproxyFactory) GetSettingsSchema() ComponentBaseSettingsAware {
	return &ToxiproxySettings{}
}

func (f *toxiproxyFactory) ConfigureContainer(settings interface{}) *containerConfig {
	s := settings.(*ToxiproxySettings)

	return &containerConfig{
		Repository: "shopify/toxiproxy",
		Tag:        "2.1.4",
		PortBindings: portBindings{
			"8474/tcp":  s.Port,
			"56248/tcp": 0,
		},
		ExposedPorts: []string{"56248"},
		ExpireAfter:  s.ExpireAfter,
	}
}

func (f *toxiproxyFactory) HealthCheck(_ interface{}) ComponentHealthCheck {
	return func(container *container) error {
		client := f.Client(container)
		_, err := client.Proxies()

		return err
	}
}

func (f *toxiproxyFactory) Component(_ cfg.Config, _ mon.Logger, container *container, _ interface{}) (Component, error) {
	binding := container.bindings["56248/tcp"]
	address := fmt.Sprintf("%s:%s", binding.host, binding.port)

	component := &ToxiproxyComponent{
		address: f.address(container),
		client:  f.Client(container),
		Bla:     address,
	}

	return component, nil
}

func (f *toxiproxyFactory) address(container *container) string {
	binding := container.bindings["8474/tcp"]
	address := fmt.Sprintf("%s:%s", binding.host, binding.port)

	return address
}

func (f *toxiproxyFactory) Client(container *container) *toxiproxy.Client {
	address := f.address(container)

	client := toxiproxy.NewClient(address)

	return client
}
