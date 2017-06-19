package openstack

import (
	"os"

	"github.com/golang/glog"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/identity/v2/tenants"
	"github.com/gophercloud/gophercloud/openstack/identity/v2/users"
	"github.com/gophercloud/gophercloud/pagination"
	gcfg "gopkg.in/gcfg.v1"
)

type Client struct {
	Identity *gophercloud.ServiceClient
	Provider *gophercloud.ProviderClient
}

type Config struct {
	Global struct {
		AuthUrl    string `gcfg:"auth-url"`
		Username   string `gcfg:"username"`
		Password   string `gcfg: "password"`
		TenantName string `gcfg:"tenant-name"`
	}
}

func toAuthOptions(cfg Config) gophercloud.AuthOptions {
	return gophercloud.AuthOptions{
		IdentityEndpoint: cfg.Global.AuthUrl,
		Username:         cfg.Global.Username,
		Password:         cfg.Global.Password,
		TenantName:       cfg.Global.TenantName,
	}
}

func NewClient(config string) (*Client, error) {
	var opts gophercloud.AuthOptions
	if cfg, err := readConfig(config); err != nil {
		glog.V(0).Info("Failed read cloudconfig: %v. Starting init openstackclient form env", err)
		opts, err = openstack.AuthOptionsFromEnv()
		if err != nil {
			return nil, err
		}
	} else {
		opts = toAuthOptions(cfg)
	}

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		return nil, err
	}

	identity, err := openstack.NewIdentityV2(provider, gophercloud.EndpointOpts{
		Availability: gophercloud.AvailabilityAdmin,
	})
	if err != nil {
		return nil, err
	}

	client := &Client{
		Identity: identity,
		Provider: provider,
	}
	return client, nil
}

func readConfig(config string) (Config, error) {
	conf, err := os.Open(config)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	err = gcfg.ReadInto(&cfg, conf)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Client) getTenantID(tenantName string) (string, error) {
	var tenantID string
	err := tenants.List(c.Identity, nil).EachPage(func(page pagination.Page) (bool, error) {
		tenantList, err1 := tenants.ExtractTenants(page)
		if err1 != nil {
			return false, err1
		}
		for _, t := range tenantList {
			if t.Name == tenantName {
				tenantID = t.ID
				break
			}
		}
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

func (c *Client) CreateTenant(tenantName string) (string, error) {
	createOpts := tenants.CreateOpts{
		Name:        tenantName,
		Description: "stackube",
		Enabled:     gophercloud.Enabled,
	}

	tenant, err := tenants.Create(c.Identity, createOpts).Extract()
	//TODO : check whether the err is aready exist or not.
	if err != nil {
		glog.Errorf("Failed to create tenant %s: %v", tenantName, err)
		return "", err
	}
	glog.V(1).Info("Tenant %s created: %v", tenantName, tenant)
	return tenant.ID, nil
}

func (c *Client) DeleteTenant(tenantName string) error {
	return tenants.List(c.Identity, nil).EachPage(func(page pagination.Page) (bool, error) {
		tenantList, err1 := tenants.ExtractTenants(page)
		if err1 != nil {
			return false, err1
		}
		for _, t := range tenantList {
			if t.Name == tenantName {
				re := tenants.Delete(c.Identity, t.ID)
				glog.V(4).Infof("Tenant %s deleted: %v", tenantName, re)
				break
			}
		}
		return true, nil
	})
}

func (c *Client) CreateUser(username, password, tenantID string) error {
	opts := users.CreateOpts{
		Name:     username,
		TenantID: tenantID,
		Enabled:  gophercloud.Enabled,
		Password: password,
	}
	user, err := users.Create(c.Identity, opts).Extract()
	if err != nil {
		glog.Errorf("Failed to create user %s: %v", username, err)
		return err
	}
	glog.V(1).Info("User %s created: %v", username, user)
	return nil
}

func (c *Client) DeleteAllUsersOnTenant(tenantName string) error {
	tenantID, err := c.getTenantID(tenantName)
	if err != nil {
		return nil
	}
	// TODO the users.List method returned users have empty TenantID option
	return users.List(c.Identity).EachPage(func(page pagination.Page) (bool, error) {
		usersList, err := users.ExtractUsers(page)
		if err != nil {
			return false, err
		}
		for _, u := range usersList {
			if u.TenantID == tenantID {
				res := users.Delete(c.Identity, u.ID)
				glog.V(4).Infof("User %s deleted: %v", u.Name, res)
			}
		}
		return true, nil
	})
}
