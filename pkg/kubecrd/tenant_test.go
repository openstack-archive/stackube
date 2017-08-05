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
	"reflect"
	"testing"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"

	"github.com/stretchr/testify/assert"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsclientfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createTenantCRD(clientset apiextensionsclient.Interface) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
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

	return nil, nil
}

func TestCreateTenantCRD(t *testing.T) {
	clientset := apiextensionsclientfake.NewSimpleClientset()

	_, err := createTenantCRD(clientset)
	assert.NoError(t, err)

	tenantCRD, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(tenantCRDName, metav1.GetOptions{})
	if err != nil {
		panic(fmt.Errorf("CustomResourceDefinitions.Create: %+v", err))
	}

	assert.Equal(t, tenantCRDName, tenantCRD.ObjectMeta.Name)
	assert.Equal(t, "tenants", tenantCRD.Spec.Names.Plural)
	assert.Equal(t, "stackube.kubernetes.io", tenantCRD.Spec.Group)
	assert.Equal(t, "v1", tenantCRD.Spec.Version)
	assert.Equal(t, apiextensionsv1beta1.NamespaceScoped, tenantCRD.Spec.Scope)
}
