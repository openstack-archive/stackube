package openstack

import (
	"errors"
	"fmt"
	"os"

	drivertypes "git.openstack.org/openstack/stackube/pkg/openstack/types"
	"git.openstack.org/openstack/stackube/pkg/util"
	"github.com/docker/distribution/uuid"
	"github.com/golang/glog"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/identity/v2/tenants"
	"github.com/gophercloud/gophercloud/openstack/identity/v2/users"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/portsbinding"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/security/groups"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/security/rules"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/pagination"
	gcfg "gopkg.in/gcfg.v1"
)

const (
	StatusCodeAlreadyExists int = 409

	podNamePrefix     = "kube"
	securitygroupName = "kube-securitygroup-default"
	HostnameMaxLen    = 63

	// Service affinities
	ServiceAffinityNone     = "None"
	ServiceAffinityClientIP = "ClientIP"
)

var (
	adminStateUp = true

	ErrNotFound        = errors.New("NotFound")
	ErrMultipleResults = errors.New("MultipleResults")
)

type Client struct {
	Identity          *gophercloud.ServiceClient
	Provider          *gophercloud.ProviderClient
	Network           *gophercloud.ServiceClient
	Region            string
	ExtNetID          string
	PluginName        string
	IntegrationBridge string
}

type PluginOpts struct {
	PluginName        string `gcfg:"plugin-name"`
	IntegrationBridge string `gcfg:"integration-bridge"`
}

type Config struct {
	Global struct {
		AuthUrl    string `gcfg:"auth-url"`
		Username   string `gcfg:"username"`
		Password   string `gcfg: "password"`
		TenantName string `gcfg:"tenant-name"`
		Region     string `gcfg:"region"`
		ExtNetID   string `gcfg:"ext-net-id"`
	}
	Plugin PluginOpts
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
	cfg, err := readConfig(config)
	if err != nil {
		glog.V(0).Infof("Failed read cloudconfig: %v. Starting init openstackclient from env", err)
		opts, err = openstack.AuthOptionsFromEnv()
		if err != nil {
			return nil, err
		}
	} else {
		opts = toAuthOptions(cfg)
	}

	glog.V(1).Infof("Initializing openstack client with config %v", cfg)

	if cfg.Global.ExtNetID == "" {
		return nil, fmt.Errorf("external network ID not set")
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

	network, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: cfg.Global.Region,
	})
	if err != nil {
		glog.Warning("Failed to find neutron endpoint: %v", err)
		return nil, err
	}

	client := &Client{
		Identity:          identity,
		Provider:          provider,
		Network:           network,
		Region:            cfg.Global.Region,
		ExtNetID:          cfg.Global.ExtNetID,
		PluginName:        cfg.Plugin.PluginName,
		IntegrationBridge: cfg.Plugin.IntegrationBridge,
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

func (c *Client) GetTenantIDFromName(tenantName string) (string, error) {
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

	_, err := tenants.Create(c.Identity, createOpts).Extract()
	if err != nil && !IsAlreadyExists(err) {
		glog.Errorf("Failed to create tenant %s: %v", tenantName, err)
		return "", err
	}
	glog.V(4).Infof("Tenant %s created", tenantName)
	tenantID, err := c.GetTenantIDFromName(tenantName)
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

func (c *Client) DeleteTenant(tenantName string) error {
	return tenants.List(c.Identity, nil).EachPage(func(page pagination.Page) (bool, error) {
		tenantList, err := tenants.ExtractTenants(page)
		if err != nil {
			return false, err
		}
		for _, t := range tenantList {
			if t.Name == tenantName {
				err := tenants.Delete(c.Identity, t.ID).ExtractErr()
				if err != nil {
					glog.Errorf("Delete openstack tenant %s error: %v", tenantName, err)
					return false, err
				}
				glog.V(4).Infof("Tenant %s deleted", tenantName)
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
	_, err := users.Create(c.Identity, opts).Extract()
	if err != nil && !IsAlreadyExists(err) {
		glog.Errorf("Failed to create user %s: %v", username, err)
		return err
	}
	glog.V(4).Infof("User %s created", username)
	return nil
}

func (c *Client) DeleteAllUsersOnTenant(tenantName string) error {
	tenantID, err := c.GetTenantIDFromName(tenantName)
	if err != nil {
		return nil
	}
	return users.ListUsers(c.Identity, tenantID).EachPage(func(page pagination.Page) (bool, error) {
		usersList, err := users.ExtractUsers(page)
		if err != nil {
			return false, err
		}
		for _, u := range usersList {
			res := users.Delete(c.Identity, u.ID)
			if res.Err != nil {
				glog.Errorf("Delete openstack user %s error: %v", u.Name, err)
				return false, err
			}
			glog.V(4).Infof("User %s deleted", u.Name)
		}
		return true, nil
	})
}

// IsAlreadyExists determines if the err is an error which indicates that a specified resource already exists.
func IsAlreadyExists(err error) bool {
	return reasonForError(err) == StatusCodeAlreadyExists
}

func reasonForError(err error) int {
	switch t := err.(type) {
	case gophercloud.ErrUnexpectedResponseCode:
		return t.Actual
	}
	return 0
}

// Get tenant's network by tenantID(tenant and network are one to one mapping in stackube)
func (os *Client) GetOpenStackNetworkByTenantID(tenantID string) (*networks.Network, error) {
	opts := networks.ListOpts{TenantID: tenantID}
	return os.getOpenStackNetwork(&opts)
}

// Get openstack network by id
func (os *Client) getOpenStackNetworkByID(id string) (*networks.Network, error) {
	opts := networks.ListOpts{ID: id}
	return os.getOpenStackNetwork(&opts)
}

// Get openstack network by name
func (os *Client) getOpenStackNetworkByName(name string) (*networks.Network, error) {
	opts := networks.ListOpts{Name: name}
	return os.getOpenStackNetwork(&opts)
}

// Get openstack network
func (os *Client) getOpenStackNetwork(opts *networks.ListOpts) (*networks.Network, error) {
	var osNetwork *networks.Network
	pager := networks.List(os.Network, *opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		networkList, e := networks.ExtractNetworks(page)
		if len(networkList) > 1 {
			return false, ErrMultipleResults
		}

		if len(networkList) == 1 {
			osNetwork = &networkList[0]
		}

		return true, e
	})

	if err == nil && osNetwork == nil {
		return nil, ErrNotFound
	}

	return osNetwork, err
}

// Get provider subnet by id
func (os *Client) GetProviderSubnet(osSubnetID string) (*drivertypes.Subnet, error) {
	s, err := subnets.Get(os.Network, osSubnetID).Extract()
	if err != nil {
		glog.Errorf("Get openstack subnet failed: %v", err)
		return nil, err
	}

	var routes []*drivertypes.Route
	for _, r := range s.HostRoutes {
		route := drivertypes.Route{
			Nexthop:         r.NextHop,
			DestinationCIDR: r.DestinationCIDR,
		}
		routes = append(routes, &route)
	}

	providerSubnet := drivertypes.Subnet{
		Uid:        s.ID,
		Cidr:       s.CIDR,
		Gateway:    s.GatewayIP,
		Name:       s.Name,
		Dnsservers: s.DNSNameservers,
		Routes:     routes,
	}

	return &providerSubnet, nil
}

// Get network by networkID
func (os *Client) GetNetworkByID(networkID string) (*drivertypes.Network, error) {
	osNetwork, err := os.getOpenStackNetworkByID(networkID)
	if err != nil {
		glog.Errorf("failed to fetch openstack network by iD: %v, failure: %v", networkID, err)
		return nil, err
	}

	return os.OSNetworktoProviderNetwork(osNetwork)
}

// Get network by networkName
func (os *Client) GetNetwork(networkName string) (*drivertypes.Network, error) {
	osNetwork, err := os.getOpenStackNetworkByName(networkName)
	if err != nil {
		glog.Warningf("try to fetch openstack network by name: %v but failed: %v", networkName, err)
		return nil, err
	}

	return os.OSNetworktoProviderNetwork(osNetwork)
}

func (os *Client) OSNetworktoProviderNetwork(osNetwork *networks.Network) (*drivertypes.Network, error) {
	var providerNetwork drivertypes.Network
	var providerSubnets []*drivertypes.Subnet
	providerNetwork.Name = osNetwork.Name
	providerNetwork.Uid = osNetwork.ID
	providerNetwork.Status = os.ToProviderStatus(osNetwork.Status)
	providerNetwork.TenantID = osNetwork.TenantID

	for _, subnetID := range osNetwork.Subnets {
		s, err := os.GetProviderSubnet(subnetID)
		if err != nil {
			return nil, err
		}
		providerSubnets = append(providerSubnets, s)
	}

	providerNetwork.Subnets = providerSubnets

	return &providerNetwork, nil
}

func (os *Client) ToProviderStatus(status string) string {
	switch status {
	case "ACTIVE":
		return "Active"
	case "BUILD":
		return "Pending"
	case "DOWN", "ERROR":
		return "Failed"
	default:
		return "Failed"
	}

	return "Failed"
}

// Create network
func (os *Client) CreateNetwork(network *drivertypes.Network) error {
	if len(network.Subnets) == 0 {
		return errors.New("Subnets is null")
	}

	// create network
	opts := networks.CreateOpts{
		Name:         network.Name,
		AdminStateUp: &adminStateUp,
		TenantID:     network.TenantID,
	}
	osNet, err := networks.Create(os.Network, opts).Extract()
	if err != nil {
		glog.Errorf("Create openstack network %s failed: %v", network.Name, err)
		return err
	}

	// create router
	routerOpts := routers.CreateOpts{
		// use network name as router name for convenience
		Name:        network.Name,
		TenantID:    network.TenantID,
		GatewayInfo: &routers.GatewayInfo{NetworkID: os.ExtNetID},
	}
	osRouter, err := routers.Create(os.Network, routerOpts).Extract()
	if err != nil {
		glog.Errorf("Create openstack router %s failed: %v", network.Name, err)
		delErr := os.DeleteNetwork(network.Name)
		if delErr != nil {
			glog.Errorf("Delete openstack network %s failed: %v", network.Name, delErr)
		}
		return err
	}

	// create subnets and connect them to router
	networkID := osNet.ID
	network.Status = os.ToProviderStatus(osNet.Status)
	network.Uid = osNet.ID
	for _, sub := range network.Subnets {
		// create subnet
		subnetOpts := subnets.CreateOpts{
			NetworkID:      networkID,
			CIDR:           sub.Cidr,
			Name:           sub.Name,
			IPVersion:      gophercloud.IPv4,
			TenantID:       network.TenantID,
			GatewayIP:      &sub.Gateway,
			DNSNameservers: sub.Dnsservers,
		}
		s, err := subnets.Create(os.Network, subnetOpts).Extract()
		if err != nil {
			glog.Errorf("Create openstack subnet %s failed: %v", sub.Name, err)
			delErr := os.DeleteNetwork(network.Name)
			if delErr != nil {
				glog.Errorf("Delete openstack network %s failed: %v", network.Name, delErr)
			}
			return err
		}

		// add subnet to router
		opts := routers.AddInterfaceOpts{
			SubnetID: s.ID,
		}
		_, err = routers.AddInterface(os.Network, osRouter.ID, opts).Extract()
		if err != nil {
			glog.Errorf("Create openstack subnet %s failed: %v", sub.Name, err)
			delErr := os.DeleteNetwork(network.Name)
			if delErr != nil {
				glog.Errorf("Delete openstack network %s failed: %v", network.Name, delErr)
			}
			return err
		}
	}

	return nil
}

// Update network
func (os *Client) UpdateNetwork(network *drivertypes.Network) error {
	// TODO: update network subnets
	return nil
}

func (os *Client) getRouterByName(name string) (*routers.Router, error) {
	var result *routers.Router

	opts := routers.ListOpts{Name: name}
	pager := routers.List(os.Network, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		routerList, e := routers.ExtractRouters(page)
		if len(routerList) > 1 {
			return false, ErrMultipleResults
		} else if len(routerList) == 1 {
			result = &routerList[0]
		}

		return true, e
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Delete network by networkName
func (os *Client) DeleteNetwork(networkName string) error {
	osNetwork, err := os.getOpenStackNetworkByName(networkName)
	if err != nil {
		glog.Errorf("Get openstack network failed: %v", err)
		return err
	}

	if osNetwork != nil {
		// Delete ports
		opts := ports.ListOpts{NetworkID: osNetwork.ID}
		pager := ports.List(os.Network, opts)
		err := pager.EachPage(func(page pagination.Page) (bool, error) {
			portList, err := ports.ExtractPorts(page)
			if err != nil {
				glog.Errorf("Get openstack ports error: %v", err)
				return false, err
			}

			for _, port := range portList {
				if port.DeviceOwner == "network:router_interface" {
					continue
				}

				err = ports.Delete(os.Network, port.ID).ExtractErr()
				if err != nil {
					glog.Warningf("Delete port %v failed: %v", port.ID, err)
				}
			}

			return true, nil
		})
		if err != nil {
			glog.Errorf("Delete ports error: %v", err)
		}

		router, err := os.getRouterByName(networkName)
		if err != nil {
			glog.Errorf("Get openstack router %s error: %v", networkName, err)
			return err
		}

		// delete all subnets
		for _, subnet := range osNetwork.Subnets {
			if router != nil {
				opts := routers.RemoveInterfaceOpts{SubnetID: subnet}
				_, err := routers.RemoveInterface(os.Network, router.ID, opts).Extract()
				if err != nil {
					glog.Errorf("Get openstack router %s error: %v", networkName, err)
					return err
				}
			}

			err = subnets.Delete(os.Network, subnet).ExtractErr()
			if err != nil {
				glog.Errorf("Delete openstack subnet %s error: %v", subnet, err)
				return err
			}
		}

		// delete router
		if router != nil {
			err = routers.Delete(os.Network, router.ID).ExtractErr()
			if err != nil {
				glog.Errorf("Delete openstack router %s error: %v", router.ID, err)
				return err
			}
		}

		// delete network
		err = networks.Delete(os.Network, osNetwork.ID).ExtractErr()
		if err != nil {
			glog.Errorf("Delete openstack network %s error: %v", osNetwork.ID, err)
			return err
		}
	}

	return nil
}

// Check the tenant id exist
func (os *Client) CheckTenantID(tenantID string) (bool, error) {
	opts := tenants.ListOpts{}
	pager := tenants.List(os.Identity, &opts)

	var found bool
	err := pager.EachPage(func(page pagination.Page) (bool, error) {

		tenantList, err := tenants.ExtractTenants(page)
		if err != nil {
			return false, err
		}

		if len(tenantList) == 0 {
			return false, ErrNotFound
		}

		for _, t := range tenantList {
			if t.ID == tenantID || t.Name == tenantID {
				found = true
			}
		}

		return true, nil
	})

	return found, err
}

func (os *Client) GetPort(name string) (*ports.Port, error) {
	opts := ports.ListOpts{Name: name}
	pager := ports.List(os.Network, opts)

	var port *ports.Port
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		portList, err := ports.ExtractPorts(page)
		if err != nil {
			glog.Errorf("Get openstack ports error: %v", err)
			return false, err
		}

		if len(portList) > 1 {
			return false, ErrMultipleResults
		}

		if len(portList) == 0 {
			return false, ErrNotFound
		}

		port = &portList[0]

		return true, err
	})

	return port, err
}

func getHostName() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}

	return host
}

func (os *Client) ensureSecurityGroup(tenantID string) (string, error) {
	var securitygroup *groups.SecGroup

	opts := groups.ListOpts{
		TenantID: tenantID,
		Name:     securitygroupName,
	}
	pager := groups.List(os.Network, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		sg, err := groups.ExtractGroups(page)
		if err != nil {
			glog.Errorf("Get openstack securitygroups error: %v", err)
			return false, err
		}

		if len(sg) > 0 {
			securitygroup = &sg[0]
		}

		return true, err
	})
	if err != nil {
		return "", err
	}

	// If securitygroup doesn't exist, create a new one
	if securitygroup == nil {
		securitygroup, err = groups.Create(os.Network, groups.CreateOpts{
			Name:     securitygroupName,
			TenantID: tenantID,
		}).Extract()

		if err != nil {
			return "", err
		}
	}

	var secGroupsRules int
	listopts := rules.ListOpts{
		TenantID:   tenantID,
		Direction:  string(rules.DirIngress),
		SecGroupID: securitygroup.ID,
	}
	rulesPager := rules.List(os.Network, listopts)
	err = rulesPager.EachPage(func(page pagination.Page) (bool, error) {
		r, err := rules.ExtractRules(page)
		if err != nil {
			glog.Errorf("Get openstack securitygroup rules error: %v", err)
			return false, err
		}

		secGroupsRules = len(r)

		return true, err
	})
	if err != nil {
		return "", err
	}

	// create new rules
	if secGroupsRules == 0 {
		// create egress rule
		_, err = rules.Create(os.Network, rules.CreateOpts{
			TenantID:   tenantID,
			SecGroupID: securitygroup.ID,
			Direction:  rules.DirEgress,
			EtherType:  rules.EtherType4,
		}).Extract()

		// create ingress rule
		_, err := rules.Create(os.Network, rules.CreateOpts{
			TenantID:   tenantID,
			SecGroupID: securitygroup.ID,
			Direction:  rules.DirIngress,
			EtherType:  rules.EtherType4,
		}).Extract()
		if err != nil {
			return "", err
		}
	}

	return securitygroup.ID, nil
}

// Create an port
func (os *Client) CreatePort(networkID, tenantID, portName string) (*portsbinding.Port, error) {
	securitygroup, err := os.ensureSecurityGroup(tenantID)
	if err != nil {
		glog.Errorf("EnsureSecurityGroup failed: %v", err)
		return nil, err
	}

	opts := portsbinding.CreateOpts{
		HostID: getHostName(),
		CreateOptsBuilder: ports.CreateOpts{
			NetworkID:      networkID,
			Name:           portName,
			AdminStateUp:   &adminStateUp,
			TenantID:       tenantID,
			DeviceID:       uuid.Generate().String(),
			DeviceOwner:    fmt.Sprintf("compute:%s", getHostName()),
			SecurityGroups: []string{securitygroup},
		},
	}

	port, err := portsbinding.Create(os.Network, opts).Extract()
	if err != nil {
		glog.Errorf("Create port %s failed: %v", portName, err)
		return nil, err
	}
	return port, nil
}

// List all ports in the network
func (os *Client) ListPorts(networkID, deviceOwner string) ([]ports.Port, error) {
	var results []ports.Port
	opts := ports.ListOpts{
		NetworkID:   networkID,
		DeviceOwner: deviceOwner,
	}
	pager := ports.List(os.Network, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		portList, err := ports.ExtractPorts(page)
		if err != nil {
			glog.Errorf("Get openstack ports error: %v", err)
			return false, err
		}

		for _, port := range portList {
			results = append(results, port)
		}

		return true, err
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

// Delete port by portName
func (os *Client) DeletePort(portName string) error {
	port, err := os.GetPort(portName)
	if err == util.ErrNotFound {
		glog.V(4).Infof("Port %s already deleted", portName)
		return nil
	} else if err != nil {
		glog.Errorf("Get openstack port %s failed: %v", portName, err)
		return err
	}

	if port != nil {
		err := ports.Delete(os.Network, port.ID).ExtractErr()
		if err != nil {
			glog.Errorf("Delete openstack port %s failed: %v", portName, err)
			return err
		}
	}

	return nil
}
