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

package tenant

import (
	"fmt"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	crdClient "git.openstack.org/openstack/stackube/pkg/kubecrd"
	"git.openstack.org/openstack/stackube/pkg/openstack"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// TenantController manages the life cycle of Tenant.
type TenantController struct {
	k8sClient       *kubernetes.Clientset
	kubeCRDClient   crdClient.Interface
	openstackClient openstack.Interface
}

// NewTenantController creates a new tenant controller.
func NewTenantController(kubeClient *kubernetes.Clientset,
	osClient openstack.Interface,
	kubeExtClient *apiextensionsclient.Clientset) (*TenantController, error) {
	// initialize CRD if it does not exist
	_, err := crdClient.CreateTenantCRD(kubeExtClient)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create CRD to kube-apiserver: %v", err)
	}

	c := &TenantController{
		kubeCRDClient:   osClient.GetCRDClient(),
		k8sClient:       kubeClient,
		openstackClient: osClient,
	}

	if err = c.createClusterRoles(); err != nil {
		return nil, fmt.Errorf("failed to create cluster roles to kube-apiserver: %v", err)
	}

	return c, nil
}

// Run the controller.
func (c *TenantController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	source := cache.NewListWatchFromClient(
		c.kubeCRDClient.Client(),
		crv1.TenantResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())

	_, tenantInformor := cache.NewInformer(
		source,
		&crv1.Tenant{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		})

	go tenantInformor.Run(stopCh)
	<-stopCh
	return nil
}

func (c *TenantController) onAdd(obj interface{}) {
	tenant := obj.(*crv1.Tenant)
	glog.V(3).Infof("Tenant controller received new object %#v\n", tenant)

	copyObj, err := c.kubeCRDClient.Scheme().Copy(tenant)
	if err != nil {
		glog.Errorf("ERROR creating a deep copy of tenant object: %#v\n", err)
		return
	}

	newTenant := copyObj.(*crv1.Tenant)
	c.syncTenant(newTenant)
}

func (c *TenantController) onUpdate(obj1, obj2 interface{}) {
	glog.Warning("tenant updates is not supported yet.")
}

func (c *TenantController) onDelete(obj interface{}) {
	tenant, ok := obj.(*crv1.Tenant)
	if !ok {
		return
	}

	glog.V(3).Infof("Tenant controller received deleted tenant %#v\n", tenant)

	deleteOptions := &apismetav1.DeleteOptions{
		TypeMeta: apismetav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
	}
	tenantName := tenant.Name
	err := c.k8sClient.Rbac().ClusterRoleBindings().Delete(tenantName+"-namespace-creater", deleteOptions)
	if err != nil && !apierrors.IsNotFound(err) {
		glog.Errorf("Failed delete ClusterRoleBinding for tenant %s: %v", tenantName, err)
	} else {
		glog.V(4).Infof("Deleted ClusterRoleBinding %s", tenantName)
	}

	// Delete automatically created network
	// TODO(harry) so that we can not deal with network with different name and namespace,
	// we need to document that.
	if err := c.kubeCRDClient.DeleteNetwork(tenantName); err != nil {
		glog.Errorf("failed to delete network for tenant: %v", tenantName)
	}

	// Delete namespace
	err = c.deleteNamespace(tenantName)
	if err != nil {
		glog.Errorf("Delete namespace %s failed: %v", tenantName, err)
	} else {
		glog.V(4).Infof("Deleted namespace %s", tenantName)
	}

	// Delete all users on a tenant
	err = c.openstackClient.DeleteAllUsersOnTenant(tenantName)
	if err != nil {
		glog.Errorf("Failed delete all users in the tenant %s: %v", tenantName, err)
	}

	// Delete tenant in keystone
	if tenant.Spec.TenantID == "" {
		err = c.openstackClient.DeleteTenant(tenantName)
		if err != nil {
			glog.Errorf("Failed delete tenant %s: %v", tenantName, err)
		}
	}
}
