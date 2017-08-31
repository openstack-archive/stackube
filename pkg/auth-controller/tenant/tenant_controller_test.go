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
	"reflect"
	"testing"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager/rbac"
	crdClient "git.openstack.org/openstack/stackube/pkg/kubecrd"
	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	password = "123456"
)

var systemTenant = &crv1.Tenant{
	ObjectMeta: apismetav1.ObjectMeta{
		Name:      util.SystemTenant,
		Namespace: util.SystemTenant,
	},
	Spec: crv1.TenantSpec{
		UserName: util.SystemTenant,
		Password: util.SystemPassword,
	},
}

func newTenant(name, userName, password, tenantID string) *crv1.Tenant {
	return &crv1.Tenant{
		ObjectMeta: apismetav1.ObjectMeta{
			Name: name,
		},
		Spec: crv1.TenantSpec{
			UserName: userName,
			Password: password,
			TenantID: tenantID,
		},
	}
}

func newNetwork(name string) *crv1.Network {
	return &crv1.Network{
		ObjectMeta: apismetav1.ObjectMeta{
			Name: name,
		},
	}
}

func newTenantController() (*TenantController, *crdClient.FakeCRDClient, *openstack.FakeOSClient, *fake.Clientset, error) {
	kubeCRDClient, err := crdClient.NewFake()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	osClient := openstack.NewFake(kubeCRDClient)

	client := fake.NewSimpleClientset()

	c := &TenantController{
		kubeCRDClient:   kubeCRDClient,
		k8sClient:       client,
		openstackClient: osClient,
	}

	if err = c.createClusterRoles(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create cluster roles to kube-apiserver: %v", err)
	}

	return c, kubeCRDClient, osClient, client, nil
}

func TestOperateNamespace(t *testing.T) {
	testNamespace := "foo"
	// Created a new fake TenantController.
	controller, _, _, client, err := newTenantController()
	if err != nil {
		t.Fatalf("Failed start a new fake TenantController")
	}

	// test create namespace
	err = controller.createNamespace(testNamespace)
	if err != nil {
		t.Fatalf("Create namespace %s error:%v", testNamespace, err)
	}
	ns, err := client.Core().Namespaces().Get(testNamespace, apismetav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get namespace %v error: %v", testNamespace, err)
	}
	if ns.Name != testNamespace {
		t.Errorf("Created namespce has incorrect parameters: %v", ns)
	}

	// test delete namespace
	err = controller.deleteNamespace(testNamespace)
	if err != nil {
		t.Fatalf("Delete namespace %v error: %v", testNamespace, err)
	}
	_, err = client.Core().Namespaces().Get(testNamespace, apismetav1.GetOptions{})
	if err.Error() != fmt.Errorf("namespaces %q not found", testNamespace).Error() {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestCreateClusterRoles(t *testing.T) {
	// Created a new fake TenantController.
	controller, _, _, client, err := newTenantController()
	if err != nil {
		t.Fatalf("Failed start a new fake TenantController")
	}
	err = controller.createClusterRoles()
	if err != nil {
		t.Fatalf("Create cluster role error:%v", err)
	}

	clusterRole, err := client.Rbac().ClusterRoles().Get("namespace-creater", apismetav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed get cluster role: %v", err)
	}

	if !reflect.DeepEqual(clusterRole, rbac.GenerateClusterRole()) {
		t.Errorf("Created cluster role has incorrect parameters: %v", clusterRole)
	}
}

func TestOnAdd(t *testing.T) {
	var controller *TenantController
	var kubeCRDClient *crdClient.FakeCRDClient
	var osClient *openstack.FakeOSClient
	var client *fake.Clientset
	var err error
	tenantID := "123"

	testCases := []struct {
		testName   string
		tenantName string
		updateFn   func(tenantName string)
		expectedFn func(tenantName string) error
	}{
		{
			testName:   "Add default Tenant",
			tenantName: "default",
			updateFn: func(tenantName string) {
				// Created a new fake TenantController.
				controller, kubeCRDClient, osClient, client, err = newTenantController()
				if err != nil {
					t.Fatalf("Failed start a new fake TenantController")
				}
				// Add default tenant
				controller.onAdd(systemTenant)

			},
			expectedFn: func(tenantName string) error {
				// test ClusterRoleBinding created
				err := testClusterRoleBindingCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				// test tenant created
				tenant, ok := osClient.Tenants[tenantName]
				if !ok {
					return fmt.Errorf("expected %s tenant to be created, got none", tenantName)
				} else if tenant.Name != tenantName {
					return fmt.Errorf("the created %s tenant has incorrect parameter: %v", tenantName, tenant)
				}
				// test user created
				user, ok := osClient.Users[tenant.ID]
				if !ok {
					return fmt.Errorf("expected %s user to be created, got none", tenantName)
				} else if user.Name != tenantName &&
					user.TenantID != tenant.ID {
					return fmt.Errorf("the created %s user has incorrect parameters: %v", tenantName, user)
				}
				// test namespace created
				err = testNamespaceCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:   "Add foo1 Tenant with no spec tenantID",
			tenantName: "foo1",
			updateFn: func(tenantName string) {
				// Add tenant
				tenant := newTenant(tenantName, tenantName, password, "")
				controller.onAdd(tenant)

			},
			expectedFn: func(tenantName string) error {
				// test ClusterRoleBinding created
				err := testClusterRoleBindingCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				// test tenant created
				tenant, ok := osClient.Tenants[tenantName]
				if !ok {
					return fmt.Errorf("expected %s tenant to be created, got none", tenantName)
				} else if tenant.Name != tenantName {
					return fmt.Errorf("the created %s tenant has incorrect parameter: %v", tenantName, tenant)
				}
				// test user created
				user, ok := osClient.Users[tenant.ID]
				if !ok {
					return fmt.Errorf("expected %s user to be created, got none", tenantName)
				} else if user.Name != tenantName &&
					user.TenantID != tenant.ID {
					return fmt.Errorf("the created %s user has incorrect parameters: %v", tenantName, user)
				}
				// test namespace created
				err = testNamespaceCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:   "Add foo2 Tenant with spec tenantID",
			tenantName: "foo2",
			updateFn: func(tenantName string) {

				tenant := newTenant(tenantName, tenantName, password, tenantID)
				// Injects fake tenant.
				osClient.SetTenant(tenantName, tenantID)

				controller.onAdd(tenant)

			},
			expectedFn: func(tenantName string) error {
				// test ClusterRoleBinding created
				err := testClusterRoleBindingCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				// test user created
				user, ok := osClient.Users[tenantID]
				if !ok {
					return fmt.Errorf("expected %s user to be created, got none", tenantName)
				} else if user.Name != tenantName &&
					user.TenantID != tenantID {
					return fmt.Errorf("the created %s user has incorrect parameters: %v", tenantName, user)
				}
				// test namespace created
				err = testNamespaceCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:   "Add foo3 Tenant, and the tenant exist in openstack",
			tenantName: "foo3",
			updateFn: func(tenantName string) {

				tenant := newTenant(tenantName, tenantName, password, "")
				// Injects fake tenant.
				osClient.SetTenant(tenantName, tenantID)

				controller.onAdd(tenant)

			},
			expectedFn: func(tenantName string) error {
				// test ClusterRoleBinding created
				err := testClusterRoleBindingCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				// test user created
				tenant, _ := osClient.Tenants[tenantName]
				user, ok := osClient.Users[tenant.ID]
				if !ok {
					return fmt.Errorf("expected %s user to be created, got none", tenantName)
				} else if user.Name != tenantName &&
					user.TenantID != tenant.ID {
					return fmt.Errorf("the created %s user has incorrect parameters: %v", tenantName, user)
				}
				// test namespace created
				err = testNamespaceCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:   "Add foo4 Tenant, and create user with error",
			tenantName: "foo4",
			updateFn: func(tenantName string) {

				tenant := newTenant(tenantName, tenantName, password, "")
				// Injects error.
				osClient.InjectError("CreateUser", fmt.Errorf("Failed create user"))

				controller.onAdd(tenant)

			},
			expectedFn: func(tenantName string) error {
				// test ClusterRoleBinding created
				err := testClusterRoleBindingCreated(t, client, tenantName)
				if err != nil {
					return err
				}
				// test no user created
				tenant, _ := osClient.Tenants[tenantName]
				user, ok := osClient.Users[tenant.ID]
				if ok {
					return fmt.Errorf("expected no user to be created, got %v", user)
				}
				return nil
			},
		},
	}

	for tci, tc := range testCases {
		tc.updateFn(tc.tenantName)
		err := tc.expectedFn(tc.tenantName)
		if err != nil {
			t.Errorf("Case[%d]: %s %v", tci, tc.testName, err)
		}
	}
}

func TestOnDelete(t *testing.T) {
	var controller *TenantController
	var kubeCRDClient *crdClient.FakeCRDClient
	var osClient *openstack.FakeOSClient
	var client *fake.Clientset
	var err error
	var tenantID string

	testCases := []struct {
		testName   string
		tenantName string
		updateFn   func(tenantName string)
		expectedFn func(tenantName string) error
	}{
		{
			testName:   "fool Tenant with no spec tenantID",
			tenantName: "foo1",
			updateFn: func(tenantName string) {
				// Created a new fake TenantController.
				controller, kubeCRDClient, osClient, client, err = newTenantController()
				if err != nil {
					t.Fatalf("Failed start a new fake TenantController")
				}

				// Injects fake network
				network := newNetwork(tenantName)
				kubeCRDClient.SetNetworks(network)
				// Add tenant
				ns := newTenant(tenantName, tenantName, password, "")
				controller.onAdd(ns)
				tenantID = osClient.Tenants[tenantName].ID
				// Delete tenant
				controller.onDelete(ns)

			},
			expectedFn: func(tenantName string) error {
				// test ClusterRoleBinding deleted
				err := testClusterRoleBindingDeleted(t, client, tenantName)
				if err != nil {
					return err
				}
				// test network deleted
				network, ok := kubeCRDClient.Networks[tenantName]
				if ok {
					return fmt.Errorf("expected %s network to be deleted, got %v", tenantName, network)
				}
				// test tenant deleted
				tenant, ok := osClient.Tenants[tenantName]
				if ok {
					return fmt.Errorf("expected %s tenant to be deleted, got %v", tenantName, tenant)
				}
				// test user deleted
				user, ok := osClient.Users[tenantID]
				if ok {
					return fmt.Errorf("expected %s user to be deleted, got %v", tenantName, user)
				}
				// test namespace deleted
				err = testNamespaceDeleted(t, client, tenantName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:   "foo2 Tenant with spec tenantID",
			tenantName: "foo2",
			updateFn: func(tenantName string) {
				// Created a new fake TenantController.
				controller, kubeCRDClient, osClient, client, err = newTenantController()
				if err != nil {
					t.Fatalf("Failed start a new fake TenantController")
				}

				// Injects fake network
				network := newNetwork(tenantName)
				kubeCRDClient.SetNetworks(network)

				tenantID = "123"
				ns := newTenant(tenantName, tenantName, password, tenantID)
				// Injects fake tenant
				osClient.SetTenant(tenantName, tenantID)
				// Add tenant
				controller.onAdd(ns)
				tenantID = osClient.Tenants[tenantName].ID
				// Delete tenant
				controller.onDelete(ns)

			},
			expectedFn: func(tenantName string) error {
				// test ClusterRoleBinding deleted
				err := testClusterRoleBindingDeleted(t, client, tenantName)
				if err != nil {
					return err
				}
				// test network deleted
				network, ok := kubeCRDClient.Networks[tenantName]
				if ok {
					return fmt.Errorf("expected %s network to be deleted, got %v", tenantName, network)
				}
				// test tenant remain existed
				_, ok = osClient.Tenants[tenantName]
				if !ok {
					return fmt.Errorf("expected %s tenant remain existed, got none", tenantName)
				}
				// test user deleted
				user, ok := osClient.Users[tenantID]
				if ok {
					return fmt.Errorf("expected %s user to be deleted, got %v", tenantName, user)
				}
				// test namespace deleted
				err = testNamespaceDeleted(t, client, tenantName)
				if err != nil {
					return err
				}
				return nil
			},
		},
	}

	for tci, tc := range testCases {
		tc.updateFn(tc.tenantName)
		err := tc.expectedFn(tc.tenantName)
		if err != nil {
			t.Errorf("Case[%d]: %s %v", tci, tc.testName, err)
		}
	}
}

func testClusterRoleBindingCreated(t *testing.T, client *fake.Clientset, tenantName string) error {
	clusterRoleBinding, err := client.Rbac().ClusterRoleBindings().Get(tenantName+"-namespace-creater", apismetav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed get ClusterRoleBinding: %v", err)
	}

	if !reflect.DeepEqual(clusterRoleBinding, rbac.GenerateClusterRoleBindingByTenant(tenantName)) {
		return fmt.Errorf("created ClusterRoleBinding has incorrect parameters: %v", clusterRoleBinding)
	}
	return nil
}

func testClusterRoleBindingDeleted(t *testing.T, client *fake.Clientset, tenantName string) error {
	_, err := client.Rbac().ClusterRoleBindings().Get(tenantName+"-namespace-creater", apismetav1.GetOptions{})

	if err.Error() != fmt.Errorf("clusterrolebindings.rbac.authorization.k8s.io %q not found", tenantName+"-namespace-creater").Error() {
		return fmt.Errorf("unexpected error: %v", err)
	}
	return nil
}

func testNamespaceCreated(t *testing.T, client *fake.Clientset, namespace string) error {
	ns, err := client.Core().Namespaces().Get(namespace, apismetav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get namespace %v error: %v", namespace, err)
	}
	if ns.Name != namespace {
		return fmt.Errorf("created namespce %v has incorrect parameters: %v", namespace, ns)
	}
	return nil
}

func testNamespaceDeleted(t *testing.T, client *fake.Clientset, namespace string) error {
	_, err := client.Core().Namespaces().Get(namespace, apismetav1.GetOptions{})
	if err.Error() != fmt.Errorf("namespaces %q not found", namespace).Error() {
		return fmt.Errorf("unexpected error: %v", err)
	}
	return nil
}
