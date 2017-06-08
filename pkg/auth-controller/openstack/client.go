package openstack

import (
	"github.com/gophercloud/gophercloud"

	"fmt"
	"os"

	"github.com/gophercloud/gophercloud/openstack"
	gcfg "gopkg.in/gcfg.v1"
)

type Client struct {
	IdentityV2 *gophercloud.ServiceClient
	IdentityV3 *gophercloud.ServiceClient
	Provider   *gophercloud.ProviderClient
}

type Config struct {
	Global struct {
		AuthUrl  string `gcfg:"auth-url"`
		Username string `gcfg:"username"`
		Password string `gcfg: "password"`
	}
}

func toAuthOptions(cfg Config) gophercloud.AuthOptions {
	return gophercloud.AuthOptions{
		IdentityEndpoint: cfg.Global.AuthUrl,
		Username:         cfg.Global.Username,
		Password:         cfg.Global.Password,
	}
}

func NewClient(config string) (*Client, error) {
	conf, err := os.Open(config)
	if err != nil {
		return nil, fmt.Errorf("init openstack client failed: %v", err)
	}
	var cfg Config
	err = gcfg.ReadInto(&cfg, conf)
	if err != nil {
		return nil, fmt.Errorf("parse openstack configure file failed: %v", err)
	}

	opts, err := openstack.AuthOptionsFromEnv()
	provider, err := openstack.AuthenticatedClient(opts)

	//provider, err := openstack.AuthenticatedClient(toAuthOptions(cfg))
	if err != nil {
		return nil, fmt.Errorf("auth openstack failed: %v", err)
	}

	identityV2, err := openstack.NewIdentityV2(provider, gophercloud.EndpointOpts{
		Availability: gophercloud.AvailabilityAdmin,
	})
	if err != nil {
		return nil, fmt.Errorf("find identity endpoint V2 failed: %v", err)
	}

	client := &Client{
		IdentityV2: identityV2,
		Provider:   provider,
	}
	return client, nil
}

func (c *Client) CreateTenant(tenant string) error {
	return nil
}

func (c *Client) DeleteTenant() error {
	return nil
}

func (c *Client) CreateUser() error {
	return nil
}

func (c *Client) DeleteUser() error {
	return nil
}

func (c *Client) GetUser() error {
	return nil
}
