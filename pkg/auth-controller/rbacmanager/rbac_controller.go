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

package rbacmanager

import (
	"time"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager/rbac"
	crdClient "git.openstack.org/openstack/stackube/pkg/kubecrd"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	resyncPeriod = 5 * time.Minute
)

// Controller manages life cycle of namespace's rbac.
type Controller struct {
	k8sclient     kubernetes.Interface
	kubeCRDClient crdClient.Interface
	userCIDR      string
	userGateway   string
}

// NewRBACController creates a new RBAC controller.
func NewRBACController(kubeClient kubernetes.Interface, kubeCRDClient crdClient.Interface, userCIDR string,
	userGateway string) (*Controller, error) {
	c := &Controller{
		k8sclient:     kubeClient,
		kubeCRDClient: kubeCRDClient,
		userCIDR:      userCIDR,
		userGateway:   userGateway,
	}

	return c, nil
}

// Run the controller.
func (c *Controller) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	source := cache.NewListWatchFromClient(
		c.k8sclient.Core().RESTClient(),
		"namespaces",
		apiv1.NamespaceAll,
		fields.Everything())

	_, namespaceInformor := cache.NewInformer(
		source,
		&apiv1.Namespace{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		})

	go namespaceInformor.Run(stopCh)
	<-stopCh
	return nil
}

func (c *Controller) onAdd(obj interface{}) {
	namespace := obj.(*apiv1.Namespace)
	glog.V(3).Infof("RBAC controller received new object %#v\n", namespace)

	// Check if this is a system reserved namespace
	if util.IsSystemNamespace(namespace.Name) {
		if err := c.initSystemReservedTenantNetwork(); err != nil {
			glog.Error(err)
			return
		}
	} else {
		if err := c.createNetworkForTenant(namespace.Name); err != nil {
			glog.Error(err)
			return
		}
	}
	glog.V(4).Infof("Added namespace %s", namespace.Name)

	c.syncRBAC(namespace)
}

// createNetworkForTenant automatically create network for given non-system tenant
func (c *Controller) createNetworkForTenant(namespace string) error {
	network := &crv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			// use the namespace name as network
			Name:      namespace,
			Namespace: namespace,
		},
		Spec: crv1.NetworkSpec{
			CIDR:    c.userCIDR,
			Gateway: c.userGateway,
		},
	}

	// network controller will always check if Tenant is ready so we will not wait here
	if err := c.kubeCRDClient.AddNetwork(network); err != nil {
		return err
	}

	return nil
}

// initSystemReservedTenantNetwork automatically create tenant network for system namespace
func (c *Controller) initSystemReservedTenantNetwork() error {
	tenant := &crv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.SystemTenant,
			// always add tenant to system namespace
			Namespace: util.SystemTenant,
		},
		Spec: crv1.TenantSpec{
			UserName: util.SystemTenant,
			Password: util.SystemPassword,
		},
	}

	if err := c.kubeCRDClient.AddTenant(tenant); err != nil {
		return err
	}

	// NOTE(harry): we do not support update Network, so although configurable,
	// user can not update CIDR by changing the configuration, unless manually delete
	// that system network. We may need to document this.
	network := &crv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.SystemNetwork,
			Namespace: util.SystemTenant,
		},
		Spec: crv1.NetworkSpec{
			CIDR:    c.userCIDR,
			Gateway: c.userGateway,
		},
	}

	// network controller will always check if Tenant is ready so we will not wait here
	if err := c.kubeCRDClient.AddNetwork(network); err != nil {
		return err
	}

	return nil
}

func (c *Controller) onUpdate(obj1, obj2 interface{}) {
	// NOTE(mozhuli) not supported yet
}

func (c *Controller) onDelete(obj interface{}) {
	namespace := obj.(*apiv1.Namespace)
	// tenant controller have done all the works so we will not wait here
	glog.V(3).Infof("RBAC controller received deleted namespace %#v\n", namespace)
}

func (c *Controller) syncRBAC(ns *apiv1.Namespace) error {
	if ns.DeletionTimestamp != nil {
		return nil
	}
	rbacClient := c.k8sclient.Rbac()

	// Create role for tenant
	role := rbac.GenerateRoleByNamespace(ns.Name)
	_, err := rbacClient.Roles(ns.Name).Create(role)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create default-role in namespace %s for tenant %s: %v", ns.Name, ns.Name, err)
		return err
	}
	glog.V(4).Infof("Created default-role in namespace %s for tenant %s", ns.Name, ns.Name)

	// Create rolebinding for tenant
	roleBinding := rbac.GenerateRoleBinding(ns.Name, ns.Name)
	_, err = rbacClient.RoleBindings(ns.Name).Create(roleBinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create %s-rolebindings in namespace %s for tenant %s: %v", ns.Name, ns.Name, ns.Name, err)
		return err
	}
	saRoleBinding := rbac.GenerateServiceAccountRoleBinding(ns.Name, ns.Name)
	_, err = rbacClient.RoleBindings(ns.Name).Create(saRoleBinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create %s-rolebindings-sa in namespace %s for tenant %s: %v", ns.Name, ns.Name, ns.Name, err)
		return err
	}

	glog.V(4).Infof("Created %s-rolebindings in namespace %s for tenant %s", ns.Name, ns.Name, ns.Name)
	return nil
}
