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
	"errors"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// FakeCRDClient is a simple fake CRD client, so that stackube
// can be run for testing without requiring a real kubernetes setup.
type FakeCRDClient struct {
	Tenants  map[string]*crv1.Tenant
	Networks map[string]*crv1.Network
	scheme   *runtime.Scheme
}

// NewFake creates a new FakeCRDClient.
func NewFake() (*FakeCRDClient, error) {
	scheme := runtime.NewScheme()
	if err := crv1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	return &FakeCRDClient{
		Tenants:  make(map[string]*crv1.Tenant),
		Networks: make(map[string]*crv1.Network),
		scheme:   scheme,
	}, nil
}

var _ = Interface(&FakeCRDClient{})

// SetTenant injects fake tenant.
func (c *FakeCRDClient) SetTenant(tenant *crv1.Tenant) {
	c.Tenants[tenant.Name] = tenant
}

// SetNetwork injects fake network.
func (c *FakeCRDClient) SetNetwork(network *crv1.Network) {
	c.Networks[network.Name] = network
}

// Client returns the RESTClient.
func (c *FakeCRDClient) Client() *rest.RESTClient {
	return nil
}

// Scheme returns runtime scheme.
func (c *FakeCRDClient) Scheme() *runtime.Scheme {
	return c.scheme
}

// AddTenant adds Tenant CRD object by given object.
func (c *FakeCRDClient) AddTenant(tenant *crv1.Tenant) error {
	if _, ok := c.Tenants[tenant.Name]; ok {
		return nil
	}

	c.Tenants[tenant.Name] = tenant
	return nil
}

// GetTenant returns Tenant CRD object by tenantName.
func (c *FakeCRDClient) GetTenant(tenantName string) (*crv1.Tenant, error) {
	tenant, ok := c.Tenants[tenantName]
	if !ok {
		return nil, errors.New("NotFound")
	}

	return tenant, nil
}

// AddNetwork adds Network CRD object by given object.
func (c *FakeCRDClient) AddNetwork(network *crv1.Network) error {
	if _, ok := c.Networks[network.Name]; ok {
		return nil
	}

	c.Networks[network.Name] = network
	return nil
}

// UpdateTenant updates Network CRD object by given object
func (c *FakeCRDClient) UpdateTenant(tenant *crv1.Tenant) {
	if _, ok := c.Tenants[tenant.Name]; !ok {
		return
	}

	c.Tenants[tenant.Name] = tenant
}

// UpdateNetwork updates Network CRD object by given object.
func (c *FakeCRDClient) UpdateNetwork(network *crv1.Network) {
	if _, ok := c.Networks[network.Name]; !ok {
		return
	}

	c.Networks[network.Name] = network
}

// DeleteNetwork deletes Network CRD object by networkName.
func (c *FakeCRDClient) DeleteNetwork(networkName string) error {
	delete(c.Networks, networkName)

	return nil
}
