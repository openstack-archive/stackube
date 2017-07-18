package client

import (
	"reflect"
	"time"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/util"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
)

const (
	tenantCRDName = crv1.TenantResourcePlural + "." + crv1.GroupName
)

func CreateTenantCRD(clientset apiextensionsclient.Interface) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: tenantCRDName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   crv1.GroupName,
			Version: crv1.SchemeGroupVersion.Version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: crv1.TenantResourcePlural,
				Kind:   reflect.TypeOf(crv1.Tenant{}).Name(),
			},
		},
	}
	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err != nil {
		return nil, err
	}

	// wait for CRD being established
	if err = util.WaitForCRDReady(clientset, tenantCRDName); err != nil {
		return nil, err
	} else {
		return crd, nil
	}
}

func WaitForTenantInstanceProcessed(tenantClient *rest.RESTClient, name string) error {
	return wait.Poll(100*time.Millisecond, 10*time.Second, func() (bool, error) {
		var tenant crv1.Tenant
		err := tenantClient.Get().
			Resource(crv1.TenantResourcePlural).
			// namespace and tenant has same name
			Namespace(name).
			Name(name).
			Do().Into(&tenant)

		if err == nil && tenant.Status.State == crv1.TenantActive {
			return true, nil
		}

		return false, err
	})
}
