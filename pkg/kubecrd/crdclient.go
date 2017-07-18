package kubecrd

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"github.com/golang/glog"
)

type CRDClient struct {
	Client *rest.RESTClient
	Scheme *runtime.Scheme
}

func NewCRDClient(cfg *rest.Config) (*CRDClient, error) {
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
		Client: client,
		Scheme: scheme,
	}, nil
}

// UpdateNetwork updates Network CRD object by given object
func (c *CRDClient) UpdateNetwork(network *crv1.Network) {
	err := c.Client.Put().
		Name(network.Name).
		Namespace(network.Namespace).
		Resource(crv1.NetworkResourcePlural).
		Body(network).
		Do().
		Error()

	if err != nil {
		glog.Errorf("ERROR updating network: %v\n", err)
	} else {
		glog.V(3).Infof("UPDATED network: %#v\n", network)
	}
}

// UpdateTenant updates Network CRD object by given object
func (c *CRDClient) UpdateTenant(tenant *crv1.Tenant) {
	err := c.Client.Put().
		Name(tenant.Name).
		Namespace(tenant.Namespace).
		Resource(crv1.TenantResourcePlural).
		Body(tenant).
		Do().
		Error()

	if err != nil {
		glog.Errorf("ERROR updating tenant: %v\n", err)
	} else {
		glog.V(3).Infof("UPDATED tenant: %#v\n", tenant)
	}
}

func (c *CRDClient) GetTenant(tenantName string) (*crv1.Tenant, error) {
	tenant := crv1.Tenant{}
	// tenant always has same name and namespace
	err := c.Client.Get().
		Resource(crv1.TenantResourcePlural).
		Namespace(tenantName).
		Name(tenantName).
		Do().Into(&tenant)
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}

func (c *CRDClient) AddTenant(tenant *crv1.Tenant) error {
	err := c.Client.Post().
		Namespace(tenant.GetNamespace()).
		Resource(crv1.TenantResourcePlural).
		Body(tenant).
		Do().Error()
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Tenant: %v", err)
	}
	return nil
}

func (c *CRDClient) AddNetwork(network *crv1.Network) error {
	err := c.Client.Post().
		Resource(crv1.NetworkResourcePlural).
		Namespace(network.GetNamespace()).
		Body(network).
		Do().Error()
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Network: %v", err)
	}
	return nil
}
