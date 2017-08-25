/*
Copyright (c) 2017 OpenStack Foundation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package openstack

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	crdClient "git.openstack.org/openstack/stackube/pkg/kubecrd"
	drivertypes "git.openstack.org/openstack/stackube/pkg/openstack/types"
	"git.openstack.org/openstack/stackube/pkg/util"
	"github.com/gophercloud/gophercloud/openstack/identity/v2/tenants"
	"github.com/gophercloud/gophercloud/openstack/identity/v2/users"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/portsbinding"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
)

var ErrAlreadyExist = errors.New("AlreadyExist")

// FakeOSClient is a simple fake openstack client, so that stackube
// can be run for testing without requiring a real openstack setup.
type FakeOSClient struct {
	Tenants           map[string]*tenants.Tenant
	Users             map[string]*users.User
	Networks          map[string]*drivertypes.Network
	Subnets           map[string]*subnets.Subnet
	Routers           map[string]*routers.Router
	Ports             map[string][]ports.Port
	CRDClient         crdClient.Interface
	PluginName        string
	IntegrationBridge string
}

var _ = Interface(&FakeOSClient{})

// NewFake creates a new FakeOSClient.
func NewFake(crdClient crdClient.Interface) *FakeOSClient {
	return &FakeOSClient{
		Tenants:           make(map[string]*tenants.Tenant),
		Users:             make(map[string]*users.User),
		Networks:          make(map[string]*drivertypes.Network),
		Subnets:           make(map[string]*subnets.Subnet),
		Routers:           make(map[string]*routers.Router),
		Ports:             make(map[string][]ports.Port),
		CRDClient:         crdClient,
		PluginName:        "ovs",
		IntegrationBridge: "bi-int",
	}
}

// SetTenant injects fake tenant.
func (os *FakeOSClient) SetTenant(tenantName, tenantID string) {
	tenant := &tenants.Tenant{
		Name: tenantName,
		ID:   tenantID,
	}
	os.Tenants[tenantName] = tenant
}

// SetUser injects fake user.
func (os *FakeOSClient) SetUser(userName, userID, tenantID string) {
	user := &users.User{
		Username: userName,
		ID:       userID,
		TenantID: tenantID,
	}
	os.Users[tenantID] = user
}

// SetNetwork injects fake network.
func (os *FakeOSClient) SetNetwork(networkName, networkID string) {
	network := &drivertypes.Network{
		Name: networkName,
		Uid:  networkID,
	}
	os.Networks[networkName] = network
}

// SetPort injects fake port.
func (os *FakeOSClient) SetPort(networkID, deviceOwner, deviceID string) {
	netPorts, ok := os.Ports[networkID]
	p := ports.Port{
		NetworkID:   networkID,
		DeviceOwner: deviceOwner,
		DeviceID:    deviceID,
	}
	if !ok {
		var ps []ports.Port
		ps = append(ps, p)
		os.Ports[networkID] = ps
	}
	netPorts = append(netPorts, p)
	os.Ports[networkID] = netPorts
}

func tenantIDHash(tenantName string) string {
	return idHash(tenantName)
}

func userIDHash(userName, tenantID string) string {
	return idHash(userName)
}

func networkIDHash(networkName string) string {
	return idHash(networkName)
}

func subnetIDHash(subnetName string) string {
	return idHash(subnetName)
}

func routerIDHash(routerName string) string {
	return idHash(routerName)
}

func portdeviceIDHash(networkID, deviceOwner string) string {
	return idHash(networkID, deviceOwner)
}

func idHash(data ...string) string {
	var s string
	for _, d := range data {
		s += d
	}
	h := sha1.New()
	io.WriteString(h, s)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// CreateTenant creates tenant by tenantname.
func (os *FakeOSClient) CreateTenant(tenantName string) (string, error) {
	if t, ok := os.Tenants[tenantName]; ok {
		return t.ID, nil
	}
	tenant := &tenants.Tenant{
		Name: tenantName,
		ID:   tenantIDHash(tenantName),
	}
	os.Tenants[tenantName] = tenant
	return tenant.ID, nil
}

// DeleteTenant deletes tenant by tenantName.
func (os *FakeOSClient) DeleteTenant(tenantName string) error {
	delete(os.Tenants, tenantName)
	return nil
}

// GetTenantIDFromName gets tenantID by tenantName.
func (os *FakeOSClient) GetTenantIDFromName(tenantName string) (string, error) {
	if util.IsSystemNamespace(tenantName) {
		tenantName = util.SystemTenant
	}

	// If tenantID is specified, return it directly
	var (
		tenant *crv1.Tenant
		err    error
	)

	if tenant, err = os.CRDClient.GetTenant(tenantName); err != nil {
		return "", err
	}
	if tenant.Spec.TenantID != "" {
		return tenant.Spec.TenantID, nil
	}

	t, ok := os.Tenants[tenantName]
	if !ok {
		return "", nil
	}

	return t.ID, nil
}

// CheckTenantByID checks tenant exist or not by tenantID.
func (os *FakeOSClient) CheckTenantByID(tenantID string) (bool, error) {
	for _, tenent := range os.Tenants {
		if tenent.ID == tenantID {
			return true, nil
		}
	}
	return false, ErrNotFound
}

// CreateUser creates user with username, password in the tenant.
func (os *FakeOSClient) CreateUser(username, password, tenantID string) error {
	user := &users.User{
		Name:     username,
		TenantID: tenantID,
		ID:       userIDHash(username, tenantID),
	}
	os.Users[tenantID] = user
	return nil
}

// DeleteAllUsersOnTenant deletes all users on the tenant.
func (os *FakeOSClient) DeleteAllUsersOnTenant(tenantName string) error {
	tenant := os.Tenants[tenantName]

	delete(os.Users, tenant.ID)
	return nil
}

func (os *FakeOSClient) createNetwork(networkName, tenantID string) error {
	if _, ok := os.Networks[networkName]; ok {
		return ErrAlreadyExist
	}

	network := &drivertypes.Network{
		Name:     networkName,
		Uid:      networkIDHash(networkName),
		TenantID: tenantID,
	}
	os.Networks[networkName] = network
	return nil
}

func (os *FakeOSClient) deleteNetwork(networkName string) error {
	delete(os.Networks, networkName)
	return nil
}

func (os *FakeOSClient) createRouter(routerName, tenantID string) error {
	if _, ok := os.Routers[routerName]; ok {
		return ErrAlreadyExist
	}

	router := &routers.Router{
		Name:     routerName,
		TenantID: tenantID,
		ID:       routerIDHash(routerName),
	}
	os.Routers[routerName] = router
	return nil
}

func (os *FakeOSClient) deleteRouter(routerName string) error {
	delete(os.Routers, routerName)
	return nil
}

func (os *FakeOSClient) createSubnet(subnetName, networkID, tenantID string) error {
	if _, ok := os.Subnets[subnetName]; ok {
		return ErrAlreadyExist
	}

	subnet := &subnets.Subnet{
		Name:      subnetName,
		TenantID:  tenantID,
		NetworkID: networkID,
		ID:        subnetIDHash(subnetName),
	}
	os.Subnets[subnetName] = subnet
	return nil
}

// CreateNetwork creates network.
// TODO(mozhuli): make it more general.
func (os *FakeOSClient) CreateNetwork(network *drivertypes.Network) error {
	if len(network.Subnets) == 0 {
		return errors.New("Subnets is null")
	}

	// create network
	err := os.createNetwork(network.Name, network.TenantID)
	if err != nil {
		return errors.New("Create network failed")
	}
	// create router, and use network name as router name for convenience.
	err = os.createRouter(network.Name, network.TenantID)
	if err != nil {
		os.deleteNetwork(network.Name)
		return errors.New("Create router failed")
	}
	// create subnets and connect them to router
	err = os.createSubnet(network.Subnets[0].Name, network.Uid, network.TenantID)
	if err != nil {
		os.deleteRouter(network.Name)
		os.deleteNetwork(network.Name)
		return errors.New("Create subnet failed")
	}
	return nil
}

// GetNetworkByID gets network by networkID.
func (os *FakeOSClient) GetNetworkByID(networkID string) (*drivertypes.Network, error) {
	return nil, nil
}

// GetNetworkByName get network by networkName
func (os *FakeOSClient) GetNetworkByName(networkName string) (*drivertypes.Network, error) {
	network, ok := os.Networks[networkName]
	if !ok {
		return nil, ErrNotFound
	}

	return network, nil
}

// DeleteNetwork deletes network by networkName.
func (os *FakeOSClient) DeleteNetwork(networkName string) error {
	return nil
}

// GetProviderSubnet gets provider subnet by id
func (os *FakeOSClient) GetProviderSubnet(osSubnetID string) (*drivertypes.Subnet, error) {
	return nil, nil
}

// CreatePort creates port by neworkID, tenantID and portName.
func (os *FakeOSClient) CreatePort(networkID, tenantID, portName string) (*portsbinding.Port, error) {
	return nil, nil
}

// GetPort gets port by portName.
func (os *FakeOSClient) GetPort(name string) (*ports.Port, error) {
	return nil, nil
}

// ListPorts list all ports which have the deviceOwner in the network.
func (os *FakeOSClient) ListPorts(networkID, deviceOwner string) ([]ports.Port, error) {
	var results []ports.Port
	portList, ok := os.Ports[networkID]
	if !ok {
		return results, nil
	}

	for _, port := range portList {
		if port.DeviceOwner == deviceOwner {
			results = append(results, port)
		}
	}
	return results, nil
}

// DeletePortByName deletes port by portName.
func (os *FakeOSClient) DeletePortByName(portName string) error {
	return nil
}

// DeletePortByID deletes port by portID.
func (os *FakeOSClient) DeletePortByID(portID string) error {
	return nil
}

// UpdatePortsBinding updates port binding.
func (os *FakeOSClient) UpdatePortsBinding(portID, deviceOwner string) error {
	return nil
}

// LoadBalancerExist returns whether a load balancer has already been exist.
func (os *FakeOSClient) LoadBalancerExist(name string) (bool, error) {
	return true, nil
}

// EnsureLoadBalancer ensures a load balancer is created.
func (os *FakeOSClient) EnsureLoadBalancer(lb *LoadBalancer) (*LoadBalancerStatus, error) {
	return nil, nil
}

// EnsureLoadBalancerDeleted ensures a load balancer is deleted.
func (os *FakeOSClient) EnsureLoadBalancerDeleted(name string) error {
	return nil
}

// GetCRDClient returns the CRDClient.
func (os *FakeOSClient) GetCRDClient() crdClient.Interface {
	return os.CRDClient
}

// GetPluginName returns the plugin name.
func (os *FakeOSClient) GetPluginName() string {
	return os.PluginName
}

// GetIntegrationBridge returns the integration bridge name.
func (os *FakeOSClient) GetIntegrationBridge() string {
	return os.IntegrationBridge
}
