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
	"fmt"
	"io"
	"sync"

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

// CalledDetail is the struct contains called function name and arguments.
type CalledDetail struct {
	// Name of the function called.
	Name string
	// Argument of the function called.
	Argument []interface{}
}

// FakeOSClient is a simple fake openstack client, so that stackube
// can be run for testing without requiring a real openstack setup.
type FakeOSClient struct {
	sync.Mutex
	called            []CalledDetail
	errors            map[string]error
	Tenants           map[string]*tenants.Tenant
	Users             map[string]*users.User
	Networks          map[string]*drivertypes.Network
	Subnets           map[string]*subnets.Subnet
	Routers           map[string]*routers.Router
	Ports             map[string][]ports.Port
	LoadBalancers     map[string]*LoadBalancer
	CRDClient         crdClient.Interface
	PluginName        string
	IntegrationBridge string
}

var _ = Interface(&FakeOSClient{})

// NewFake creates a new FakeOSClient.
func NewFake(crdClient crdClient.Interface) *FakeOSClient {
	return &FakeOSClient{
		errors:            make(map[string]error),
		Tenants:           make(map[string]*tenants.Tenant),
		Users:             make(map[string]*users.User),
		Networks:          make(map[string]*drivertypes.Network),
		Subnets:           make(map[string]*subnets.Subnet),
		Routers:           make(map[string]*routers.Router),
		Ports:             make(map[string][]ports.Port),
		LoadBalancers:     make(map[string]*LoadBalancer),
		CRDClient:         crdClient,
		PluginName:        "ovs",
		IntegrationBridge: "bi-int",
	}
}

func (f *FakeOSClient) getError(op string) error {
	err, ok := f.errors[op]
	if ok {
		delete(f.errors, op)
		return err
	}
	return nil
}

// InjectError inject error for call
func (f *FakeOSClient) InjectError(fn string, err error) {
	f.Lock()
	defer f.Unlock()
	f.errors[fn] = err
}

// InjectErrors inject errors for calls
func (f *FakeOSClient) InjectErrors(errs map[string]error) {
	f.Lock()
	defer f.Unlock()
	for fn, err := range errs {
		f.errors[fn] = err
	}
}

// ClearErrors clear errors for call
func (f *FakeOSClient) ClearErrors() {
	f.Lock()
	defer f.Unlock()
	f.errors = make(map[string]error)
}

func (f *FakeOSClient) appendCalled(name string, argument ...interface{}) {
	call := CalledDetail{Name: name, Argument: argument}
	f.called = append(f.called, call)
}

// GetCalledNames get names of call
func (f *FakeOSClient) GetCalledNames() []string {
	f.Lock()
	defer f.Unlock()
	names := []string{}
	for _, detail := range f.called {
		names = append(names, detail.Name)
	}
	return names
}

// GetCalledDetails get detail of each call.
func (f *FakeOSClient) GetCalledDetails() []CalledDetail {
	f.Lock()
	defer f.Unlock()
	// Copy the list and return.
	return append([]CalledDetail{}, f.called...)
}

// SetTenant injects fake tenant.
func (f *FakeOSClient) SetTenant(tenantName, tenantID string) {
	f.Lock()
	defer f.Unlock()
	tenant := &tenants.Tenant{
		Name: tenantName,
		ID:   tenantID,
	}
	f.Tenants[tenantName] = tenant
}

// SetUser injects fake user.
func (f *FakeOSClient) SetUser(userName, userID, tenantID string) {
	f.Lock()
	defer f.Unlock()
	user := &users.User{
		Username: userName,
		ID:       userID,
		TenantID: tenantID,
	}
	f.Users[tenantID] = user
}

// SetNetwork injects fake network.
func (f *FakeOSClient) SetNetwork(network *drivertypes.Network) {
	f.Lock()
	defer f.Unlock()

	f.Networks[network.Name] = network
}

// SetPort injects fake port.
func (f *FakeOSClient) SetPort(networkID, deviceOwner, deviceID string) {
	f.Lock()
	defer f.Unlock()
	netPorts, ok := f.Ports[networkID]
	p := ports.Port{
		NetworkID:   networkID,
		DeviceOwner: deviceOwner,
		DeviceID:    deviceID,
	}
	if !ok {
		var ps []ports.Port
		ps = append(ps, p)
		f.Ports[networkID] = ps
	}
	netPorts = append(netPorts, p)
	f.Ports[networkID] = netPorts
}

// SetLoadbalancer injects fake loadbalancer.
func (f *FakeOSClient) SetLoadbalancer(lb *LoadBalancer) {
	f.Lock()
	defer f.Unlock()

	f.LoadBalancers[lb.Name] = lb
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

// CreateTenant is a test implementation of Interface.CreateTenant.
func (f *FakeOSClient) CreateTenant(tenantName string) (string, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("CreateTenant", tenantName)
	if err := f.getError("CreateTenant"); err != nil {
		return "", err
	}

	if t, ok := f.Tenants[tenantName]; ok {
		return t.ID, nil
	}
	tenant := &tenants.Tenant{
		Name: tenantName,
		ID:   tenantIDHash(tenantName),
	}
	f.Tenants[tenantName] = tenant
	return tenant.ID, nil
}

// DeleteTenant is a test implementation of Interface.DeleteTenant.
func (f *FakeOSClient) DeleteTenant(tenantName string) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("DeleteTenant", tenantName)
	if err := f.getError("DeleteTenant"); err != nil {
		return err
	}

	delete(f.Tenants, tenantName)
	return nil
}

// GetTenantIDFromName is a test implementation of Interface.GetTenantIDFromName.
func (f *FakeOSClient) GetTenantIDFromName(tenantName string) (string, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("GetTenantIDFromName", tenantName)
	if err := f.getError("GetTenantIDFromName"); err != nil {
		return "", err
	}

	if util.IsSystemNamespace(tenantName) {
		tenantName = util.SystemTenant
	}

	// If tenantID is specified, return it directly
	tenant, err := f.CRDClient.GetTenant(tenantName)
	if err != nil {
		return "", err
	}
	if tenant.Spec.TenantID != "" {
		return tenant.Spec.TenantID, nil
	}

	t, ok := f.Tenants[tenantName]
	if !ok {
		return "", ErrNotFound
	}

	return t.ID, nil
}

// CheckTenantByID is a test implementation of Interface.CheckTenantByID.
func (f *FakeOSClient) CheckTenantByID(tenantID string) (bool, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("CheckTenantByID", tenantID)
	if err := f.getError("CheckTenantByID"); err != nil {
		return false, err
	}

	for _, tenent := range f.Tenants {
		if tenent.ID == tenantID {
			return true, nil
		}
	}
	return false, nil
}

// CreateUser is a test implementation of Interface.CreateUser.
func (f *FakeOSClient) CreateUser(username, password, tenantID string) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("CreateUser", username, password, tenantID)
	if err := f.getError("CreateUser"); err != nil {
		return err
	}

	user := &users.User{
		Name:     username,
		TenantID: tenantID,
		ID:       userIDHash(username, tenantID),
	}
	f.Users[tenantID] = user
	return nil
}

// DeleteAllUsersOnTenant is a test implementation of Interface.DeleteAllUsersOnTenant.
func (f *FakeOSClient) DeleteAllUsersOnTenant(tenantName string) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("DeleteAllUsersOnTenant", tenantName)
	if err := f.getError("DeleteAllUsersOnTenant"); err != nil {
		return err
	}

	tenant := f.Tenants[tenantName]

	delete(f.Users, tenant.ID)
	return nil
}

func (f *FakeOSClient) createNetwork(networkName, tenantID string) error {
	f.appendCalled("createNetwork", networkName, tenantID)
	if err := f.getError("createNetwork"); err != nil {
		return err
	}

	if _, ok := f.Networks[networkName]; ok {
		return fmt.Errorf("Network %s already exist", networkName)
	}

	network := &drivertypes.Network{
		Name:     networkName,
		Uid:      networkIDHash(networkName),
		TenantID: tenantID,
	}
	f.Networks[networkName] = network
	return nil
}

func (f *FakeOSClient) deleteNetwork(networkName string) error {
	f.appendCalled("deleteNetwork", networkName)
	if err := f.getError("deleteNetwork"); err != nil {
		return err
	}

	delete(f.Networks, networkName)
	return nil
}

func (f *FakeOSClient) createRouter(routerName, tenantID string) error {
	f.appendCalled("createRouter", routerName, tenantID)
	if err := f.getError("createRouter"); err != nil {
		return err
	}

	if _, ok := f.Routers[routerName]; ok {
		return fmt.Errorf("Router %s already exist", routerName)
	}

	router := &routers.Router{
		Name:     routerName,
		TenantID: tenantID,
		ID:       routerIDHash(routerName),
	}
	f.Routers[routerName] = router
	return nil
}

func (f *FakeOSClient) deleteRouter(routerName string) error {
	f.appendCalled("deleteRouter", routerName)
	if err := f.getError("deleteRouter"); err != nil {
		return err
	}

	delete(f.Routers, routerName)
	return nil
}

func (f *FakeOSClient) createSubnet(subnetName, networkID, tenantID string) error {
	f.appendCalled("createSubnet", subnetName, networkID, tenantID)
	if err := f.getError("createSubnet"); err != nil {
		return err
	}

	if _, ok := f.Subnets[subnetName]; ok {
		return fmt.Errorf("Subnet %s already exist", subnetName)
	}

	subnet := &subnets.Subnet{
		Name:      subnetName,
		TenantID:  tenantID,
		NetworkID: networkID,
		ID:        subnetIDHash(subnetName),
	}
	f.Subnets[subnetName] = subnet
	return nil
}

// CreateNetwork is a test implementation of Interface.CreateNetwork.
func (f *FakeOSClient) CreateNetwork(network *drivertypes.Network) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("CreateNetwork", network)
	if err := f.getError("CreateNetwork"); err != nil {
		return err
	}

	if len(network.Subnets) == 0 {
		return fmt.Errorf("Subnets is null")
	}

	// create network
	err := f.createNetwork(network.Name, network.TenantID)
	if err != nil {
		return err
	}
	// create router, and use network name as router name for convenience.
	err = f.createRouter(network.Name, network.TenantID)
	if err != nil {
		f.deleteNetwork(network.Name)
		return err
	}
	// create subnets and connect them to router
	err = f.createSubnet(network.Subnets[0].Name, network.Uid, network.TenantID)
	if err != nil {
		f.deleteRouter(network.Name)
		f.deleteNetwork(network.Name)
		return err
	}
	return nil
}

// GetNetworkByID is a test implementation of Interface.GetNetworkByID.
func (f *FakeOSClient) GetNetworkByID(networkID string) (*drivertypes.Network, error) {
	for _, network := range f.Networks {
		if network.Uid == networkID {
			return network, nil
		}
	}
	return nil, ErrNotFound
}

// GetNetworkByName is a test implementation of Interface.GetNetworkByName.
func (f *FakeOSClient) GetNetworkByName(networkName string) (*drivertypes.Network, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("GetNetworkByName", networkName)
	if err := f.getError("GetNetworkByName"); err != nil {
		return nil, err
	}

	network, ok := f.Networks[networkName]
	if !ok {
		return nil, ErrNotFound
	}

	return network, nil
}

// DeleteNetwork is a test implementation of Interface.DeleteNetwork.
func (f *FakeOSClient) DeleteNetwork(networkName string) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("DeleteNetwork", networkName)
	if err := f.getError("DeleteNetwork"); err != nil {
		return err
	}

	delete(f.Networks, networkName)
	return nil
}

// GetProviderSubnet is a test implementation of Interface.GetProviderSubnet.
func (f *FakeOSClient) GetProviderSubnet(osSubnetID string) (*drivertypes.Subnet, error) {
	return nil, fmt.Errorf("Not implemented")
}

// CreatePort is a test implementation of Interface.CreatePort.
func (f *FakeOSClient) CreatePort(networkID, tenantID, portName string) (*portsbinding.Port, error) {
	return nil, fmt.Errorf("Not implemented")
}

// GetPort is a test implementation of Interface.GetPort.
func (f *FakeOSClient) GetPort(name string) (*ports.Port, error) {
	return nil, fmt.Errorf("Not implemented")
}

// ListPorts is a test implementation of Interface.ListPorts.
func (f *FakeOSClient) ListPorts(networkID, deviceOwner string) ([]ports.Port, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("ListPorts", networkID, deviceOwner)
	if err := f.getError("ListPorts"); err != nil {
		return nil, err
	}

	var results []ports.Port
	portList, ok := f.Ports[networkID]
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

// DeletePortByName is a test implementation of Interface.DeletePortByName.
func (f *FakeOSClient) DeletePortByName(portName string) error {
	return fmt.Errorf("Not implemented")
}

// DeletePortByID is a test implementation of Interface.DeletePortByID.
func (f *FakeOSClient) DeletePortByID(portID string) error {
	return fmt.Errorf("Not implemented")
}

// UpdatePortsBinding is a test implementation of Interface.UpdatePortsBinding.
func (f *FakeOSClient) UpdatePortsBinding(portID, deviceOwner string) error {
	return fmt.Errorf("Not implemented")
}

// LoadBalancerExist is a test implementation of Interface.LoadBalancerExist.
func (f *FakeOSClient) LoadBalancerExist(name string) (bool, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("LoadBalancerExist", name)
	if err := f.getError("LoadBalancerExist"); err != nil {
		return false, err
	}

	if _, ok := f.LoadBalancers[name]; !ok {
		return false, nil
	}

	return true, nil
}

// EnsureLoadBalancer is a test implementation of Interface.EnsureLoadBalancer.
func (f *FakeOSClient) EnsureLoadBalancer(lb *LoadBalancer) (*LoadBalancerStatus, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("EnsureLoadBalancer", lb)
	if err := f.getError("EnsureLoadBalancer"); err != nil {
		return nil, err
	}

	f.LoadBalancers[lb.Name] = lb

	return &LoadBalancerStatus{
		InternalIP: lb.InternalIP,
		ExternalIP: lb.ExternalIP,
	}, nil
}

// EnsureLoadBalancerDeleted is a test implementation of Interface.EnsureLoadBalancerDeleted.
func (f *FakeOSClient) EnsureLoadBalancerDeleted(name string) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("EnsureLoadBalancerDeleted", name)
	if err := f.getError("EnsureLoadBalancerDeleted"); err != nil {
		return err
	}

	delete(f.LoadBalancers, name)
	return nil
}

// GetCRDClient is a test implementation of Interface.GetCRDClient.
func (f *FakeOSClient) GetCRDClient() crdClient.Interface {
	return f.CRDClient
}

// GetPluginName is a test implementation of Interface.GetPluginName.
func (f *FakeOSClient) GetPluginName() string {
	return f.PluginName
}

// GetIntegrationBridge is a test implementation of Interface.GetIntegrationBridge.
func (f *FakeOSClient) GetIntegrationBridge() string {
	return f.IntegrationBridge
}
