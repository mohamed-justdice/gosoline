package env

import (
	toxiproxy "github.com/Shopify/toxiproxy/client"
	"github.com/applike/gosoline/pkg/cfg"
)

type ToxiproxyComponent struct {
	baseComponent
	address string
	client  *toxiproxy.Client
	Bla     string
}

func (c *ToxiproxyComponent) CfgOptions() []cfg.Option {
	return []cfg.Option{}
}

func (c *ToxiproxyComponent) Address() string {
	return c.address
}

func (c *ToxiproxyComponent) Client() *toxiproxy.Client {
	return c.client
}

func (c *ToxiproxyComponent) ProxyForComponent() *toxiproxy.Client {
	return c.client
}
