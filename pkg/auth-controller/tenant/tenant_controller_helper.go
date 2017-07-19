package tenant

import (
	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager/rbac"
	"git.openstack.org/openstack/stackube/pkg/openstack"

	"github.com/golang/glog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

func (c *TenantController) syncTenant(tenant *crv1.Tenant) {
	roleBinding := rbac.GenerateClusterRoleBindingByTenant(tenant.Name)
	_, err := c.k8sClient.Rbac().ClusterRoleBindings().Create(roleBinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create ClusterRoleBinding for tenant %s: %v", tenant.Name, err)
		return
	}
	glog.V(4).Infof("Created ClusterRoleBindings %s-namespace-creater for tenant %s", tenant.Name, tenant.Name)
	if tenant.Spec.TenantID != "" {
		// Create user with the spec username and password in the given tenant
		err = c.openstackClient.CreateUser(tenant.Spec.UserName, tenant.Spec.Password, tenant.Spec.TenantID)
		if err != nil && !openstack.IsAlreadyExists(err) {
			glog.Errorf("Failed create user %s: %v", tenant.Spec.UserName, err)
			return
		}
	} else {
		// Create tenant if the tenant not exist in keystone
		tenantID, err := c.openstackClient.CreateTenant(tenant.Name)
		if err != nil {
			glog.Errorf("Failed create tenant %s: %v", tenant, err)
			return
		}
		// Create user with the spec username and password in the created tenant
		err = c.openstackClient.CreateUser(tenant.Spec.UserName, tenant.Spec.Password, tenantID)
		if err != nil {
			glog.Errorf("Failed create user %s: %v", tenant.Spec.UserName, err)
			return
		}
	}

	// Create namespace which name is the same as the tenant's name
	err = c.createNamespace(tenant.Name)
	if err != nil {
		glog.Errorf("Failed create namespace %s: %v", tenant.Name, err)
		return
	}
	glog.V(4).Infof("Created namespace %s for tenant %s", tenant.Name, tenant.Name)
}

func (c *TenantController) createClusterRoles() error {
	nsCreater := rbac.GenerateClusterRole()
	_, err := c.k8sClient.Rbac().ClusterRoles().Create(nsCreater)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create ClusterRoles namespace-creater: %v", err)
		return err
	}
	glog.V(4).Info("Created ClusterRoles namespace-creater")
	return nil
}

func (c *TenantController) createNamespace(namespace string) error {
	_, err := c.k8sClient.CoreV1().Namespaces().Create(&apiv1.Namespace{
		ObjectMeta: apismetav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create namespace %s: %v", namespace, err)
		return err
	}
	return nil
}

func (c *TenantController) deleteNamespace(namespace string) error {
	err := c.k8sClient.CoreV1().Namespaces().Delete(namespace, apismetav1.NewDeleteOptions(0))
	if err != nil {
		return err
	}
	return nil
}
