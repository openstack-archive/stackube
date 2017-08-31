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

package kubecrd

import (
	"fmt"
	"sync"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// CalledDetail is the struct contains called function name and arguments.
type CalledDetail struct {
	// Name of the function called.
	Name string
	// Argument of the function called.
	Argument interface{}
}

// FakeCRDClient is a simple fake CRD client, so that stackube
// can be run for testing without requiring a real kubernetes setup.
type FakeCRDClient struct {
	sync.Mutex
	called   []CalledDetail
	errors   map[string]error
	Tenants  map[string]*crv1.Tenant
	Networks map[string]*crv1.Network
	scheme   *runtime.Scheme
}

var _ = Interface(&FakeCRDClient{})

// NewFake creates a new FakeCRDClient.
func NewFake() (*FakeCRDClient, error) {
	scheme := runtime.NewScheme()
	if err := crv1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	return &FakeCRDClient{
		errors:   make(map[string]error),
		Tenants:  make(map[string]*crv1.Tenant),
		Networks: make(map[string]*crv1.Network),
		scheme:   scheme,
	}, nil
}

func (f *FakeCRDClient) getError(op string) error {
	err, ok := f.errors[op]
	if ok {
		delete(f.errors, op)
		return err
	}
	return nil
}

// InjectError inject error for call
func (f *FakeCRDClient) InjectError(fn string, err error) {
	f.Lock()
	defer f.Unlock()
	f.errors[fn] = err
}

// InjectErrors inject errors for calls
func (f *FakeCRDClient) InjectErrors(errs map[string]error) {
	f.Lock()
	defer f.Unlock()
	for fn, err := range errs {
		f.errors[fn] = err
	}
}

// ClearErrors clear errors for call
func (f *FakeCRDClient) ClearErrors() {
	f.Lock()
	defer f.Unlock()
	f.errors = make(map[string]error)
}

func (f *FakeCRDClient) appendCalled(name string, argument interface{}) {
	call := CalledDetail{Name: name, Argument: argument}
	f.called = append(f.called, call)
}

// GetCalledNames get names of call
func (f *FakeCRDClient) GetCalledNames() []string {
	f.Lock()
	defer f.Unlock()
	names := []string{}
	for _, detail := range f.called {
		names = append(names, detail.Name)
	}
	return names
}

// GetCalledDetails get detail of each call.
func (f *FakeCRDClient) GetCalledDetails() []CalledDetail {
	f.Lock()
	defer f.Unlock()
	// Copy the list and return.
	return append([]CalledDetail{}, f.called...)
}

// SetTenants injects fake tenant.
func (f *FakeCRDClient) SetTenants(tenants ...*crv1.Tenant) {
	f.Lock()
	defer f.Unlock()
	for _, tenant := range tenants {
		f.Tenants[tenant.Name] = tenant
	}
}

// SetNetworks injects fake network.
func (f *FakeCRDClient) SetNetworks(networks ...*crv1.Network) {
	f.Lock()
	defer f.Unlock()
	for _, network := range networks {
		f.Networks[network.Name] = network
	}
}

// Client is a test implementation of Interface.Client.
func (f *FakeCRDClient) Client() *rest.RESTClient {
	return nil
}

// Scheme is a test implementation of Interface.Scheme.
func (f *FakeCRDClient) Scheme() *runtime.Scheme {
	return f.scheme
}

// AddTenant is a test implementation of Interface.AddTenant.
func (f *FakeCRDClient) AddTenant(tenant *crv1.Tenant) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("AddTenant", tenant)
	if err := f.getError("AddTenant"); err != nil {
		return err
	}

	if _, ok := f.Tenants[tenant.Name]; ok {
		return nil
	}

	f.Tenants[tenant.Name] = tenant
	return nil
}

// GetTenant is a test implementation of Interface.GetTenant.
func (f *FakeCRDClient) GetTenant(tenantName string) (*crv1.Tenant, error) {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("GetTenant", tenantName)
	if err := f.getError("GetTenant"); err != nil {
		return nil, err
	}

	tenant, ok := f.Tenants[tenantName]
	if !ok {
		return nil, fmt.Errorf("Tenant %s not found", tenantName)
	}

	return tenant, nil
}

// DeleteTenant is a test implementation of Interface.DeleteTenant.
func (f *FakeCRDClient) DeleteTenant(tenantName string) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("DeleteTenant", tenantName)
	if err := f.getError("DeleteTenant"); err != nil {
		return err
	}

	delete(f.Tenants, tenantName)

	return nil
}

// AddNetwork is a test implementation of Interface.AddNetwork.
func (f *FakeCRDClient) AddNetwork(network *crv1.Network) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("AddNetwork", network)
	if err := f.getError("AddNetwork"); err != nil {
		return err
	}

	if _, ok := f.Networks[network.Name]; ok {
		return nil
	}

	f.Networks[network.Name] = network
	return nil
}

// UpdateTenant is a test implementation of Interface.UpdateTenant.
func (f *FakeCRDClient) UpdateTenant(tenant *crv1.Tenant) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("UpdateTenant", tenant)
	if err := f.getError("UpdateTenant"); err != nil {
		return err
	}

	if _, ok := f.Tenants[tenant.Name]; !ok {
		return fmt.Errorf("Tenant %s not exist", tenant.Name)
	}

	f.Tenants[tenant.Name] = tenant
	return nil
}

// UpdateNetwork is a test implementation of Interface.UpdateNetwork.
func (f *FakeCRDClient) UpdateNetwork(network *crv1.Network) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("UpdateNetwork", network)
	if err := f.getError("UpdateNetwork"); err != nil {
		return err
	}

	if _, ok := f.Networks[network.Name]; !ok {
		return fmt.Errorf("Network %s not exist", network.Name)
	}

	f.Networks[network.Name] = network
	return nil
}

// DeleteNetwork is a test implementation of Interface.DeleteNetwork.
func (f *FakeCRDClient) DeleteNetwork(networkName string) error {
	f.Lock()
	defer f.Unlock()
	f.appendCalled("DeleteNetwork", networkName)
	if err := f.getError("DeleteNetwork"); err != nil {
		return err
	}

	delete(f.Networks, networkName)

	return nil
}
