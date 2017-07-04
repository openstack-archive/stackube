package auth

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/rest"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
)

var (
	CRDGroup   = crv1.GroupName
	CRDVersion = crv1.SchemeGroupVersion.Version
)

type AuthInterface interface {
	RESTClient() rest.Interface
	TenantsGetter
	//TODO: add networkgetter
}

type AuthClient struct {
	restClient    rest.Interface
	dynamicClient *dynamic.Client
}

func (c *AuthClient) Tenants(namespace string) TenantInterface {
	return newTenants(c.restClient, c.dynamicClient, namespace)
}

func (c *AuthClient) RESTClient() rest.Interface {
	return c.restClient
}

func NewForConfig(c *rest.Config) (*AuthClient, error) {
	config := *c
	setConfigDefaults(&config)
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewClient(&config)
	if err != nil {
		return nil, err
	}

	return &AuthClient{client, dynamicClient}, nil
}

func setConfigDefaults(config *rest.Config) {
	config.GroupVersion = &schema.GroupVersion{
		Group:   CRDGroup,
		Version: CRDVersion,
	}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}
	return
}
