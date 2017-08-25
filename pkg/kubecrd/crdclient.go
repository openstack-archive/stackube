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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
)

// Interface should be implemented by a CRD client.
type Interface interface {
	// AddTenant adds Tenant CRD object by given object.
	AddTenant(tenant *crv1.Tenant) error
	// GetTenant returns Tenant CRD object by tenantName.
	GetTenant(tenantName string) (*crv1.Tenant, error)
	// UpdateTenant updates Tenant CRD object by given object.
	UpdateTenant(tenant *crv1.Tenant) error
	// AddNetwork adds Network CRD object by given object.
	AddNetwork(network *crv1.Network) error
	// UpdateNetwork updates Network CRD object by given object.
	UpdateNetwork(network *crv1.Network) error
	// DeleteNetwork deletes Network CRD object by networkName.
	DeleteNetwork(networkName string) error
	// Client returns the RESTClient.
	Client() *rest.RESTClient
	// Scheme returns runtime scheme.
	Scheme() *runtime.Scheme
}

// CRDClient implements the Interface.
type CRDClient struct {
	client *rest.RESTClient
	scheme *runtime.Scheme
}

// NewCRDClient returns a new CRD client.
func NewCRDClient(cfg *rest.Config) (Interface, error) {
	scheme := runtime.NewScheme()
	if err := crv1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	config := *cfg
	config.GroupVersion = &crv1.SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &CRDClient{
		client: client,
		scheme: scheme,
	}, nil
}

// Client returns the RESTClient.
func (c *CRDClient) Client() *rest.RESTClient {
	return c.client
}

// Scheme returns runtime scheme.
func (c *CRDClient) Scheme() *runtime.Scheme {
	return c.scheme
}

// UpdateNetwork updates Network CRD object by given object.
func (c *CRDClient) UpdateNetwork(network *crv1.Network) error {
	err := c.client.Put().
		Name(network.Name).
		Namespace(network.Namespace).
		Resource(crv1.NetworkResourcePlural).
		Body(network).
		Do().
		Error()

	if err != nil {
		glog.Errorf("ERROR updating network: %v\n", err)
		return err
	}
	glog.V(3).Infof("UPDATED network: %#v\n", network)
	return nil
}

// UpdateTenant updates Network CRD object by given object.
func (c *CRDClient) UpdateTenant(tenant *crv1.Tenant) error {
	err := c.client.Put().
		Name(tenant.Name).
		Namespace(util.SystemTenant).
		Resource(crv1.TenantResourcePlural).
		Body(tenant).
		Do().
		Error()

	if err != nil {
		glog.Errorf("ERROR updating tenant: %v\n", err)
		return err
	}
	glog.V(3).Infof("UPDATED tenant: %#v\n", tenant)
	return nil
}

// GetTenant returns Tenant CRD object by tenantName.
// NOTE: all tenant are stored under system namespace.
func (c *CRDClient) GetTenant(tenantName string) (*crv1.Tenant, error) {
	tenant := crv1.Tenant{}
	// tenant always has the same name with namespace
	err := c.client.Get().
		Resource(crv1.TenantResourcePlural).
		Namespace(util.SystemTenant).
		Name(tenantName).
		Do().Into(&tenant)
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}

// AddTenant adds Tenant CRD object by given object.
// NOTE: all tenant are added to system namespace.
func (c *CRDClient) AddTenant(tenant *crv1.Tenant) error {
	err := c.client.Post().
		Namespace(util.SystemTenant).
		Resource(crv1.TenantResourcePlural).
		Body(tenant).
		Do().Error()
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Tenant: %v", err)
	}
	return nil
}

// AddNetwork adds Network CRD object by given object.
func (c *CRDClient) AddNetwork(network *crv1.Network) error {
	err := c.client.Post().
		Resource(crv1.NetworkResourcePlural).
		Namespace(network.GetNamespace()).
		Body(network).
		Do().Error()
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Network: %v", err)
	}
	return nil
}

// DeleteNetwork deletes Network CRD object by networkName.
// NOTE: the automatically created network for tenant use namespace as name.
func (c *CRDClient) DeleteNetwork(networkName string) error {
	err := c.client.Delete().
		Resource(crv1.NetworkResourcePlural).
		Namespace(networkName).
		Name(networkName).
		Do().Error()
	if err != nil {
		return fmt.Errorf("failed to delete Network: %v", err)
	}
	return nil
}
