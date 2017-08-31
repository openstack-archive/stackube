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

package network

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	crdClient "git.openstack.org/openstack/stackube/pkg/kubecrd"
	"git.openstack.org/openstack/stackube/pkg/openstack"
	drivertypes "git.openstack.org/openstack/stackube/pkg/openstack/types"
	"git.openstack.org/openstack/stackube/pkg/util"

	apiv1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	userCIDR    = "10.244.0.0/16"
	userGateway = "10.244.0.1"
	password    = "123456"
	tenantID    = "123"
	networkID   = "456"
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

func newNetwork(networkName, networkID string) *crv1.Network {
	return &crv1.Network{
		ObjectMeta: apismetav1.ObjectMeta{
			Name:      networkName,
			Namespace: networkName,
		},
		Spec: crv1.NetworkSpec{
			CIDR:      userCIDR,
			Gateway:   userGateway,
			NetworkID: networkID,
		},
	}
}

func osNetwork(networkName, tenantID, networkID string) *drivertypes.Network {
	return &drivertypes.Network{
		Name:     networkName,
		TenantID: tenantID,
		Uid:      networkID,
	}
}

func newTenant(name, tenantID string) *crv1.Tenant {
	return &crv1.Tenant{
		ObjectMeta: apismetav1.ObjectMeta{
			Name: name,
		},
		Spec: crv1.TenantSpec{
			TenantID: tenantID,
		},
	}
}

func newNetworkController() (*NetworkController, *crdClient.FakeCRDClient, *openstack.FakeOSClient, *fake.Clientset, error) {
	kubeCRDClient, err := crdClient.NewFake()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	osClient := openstack.NewFake(kubeCRDClient)

	client := fake.NewSimpleClientset()

	c := &NetworkController{
		k8sclient:     client,
		kubeCRDClient: kubeCRDClient,
		driver:        osClient,
	}

	return c, kubeCRDClient, osClient, client, nil
}

func TestCreateKubeDNSDeployment(t *testing.T) {
	testNamespace := "foo"
	// Created a new fake NetworkController.
	controller, _, _, client, err := newNetworkController()
	if err != nil {
		t.Fatalf("Failed start a new fake NetworkController")
	}

	err = controller.createKubeDNSDeployment(testNamespace)
	if err != nil {
		t.Fatalf("Create kube-dns deployment in namespace %v error: %v", testNamespace, err)
	}

	kubeDNSDeploy, err := client.ExtensionsV1beta1().Deployments(testNamespace).Get("kube-dns", apismetav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed get kube-dns deployment in namespace %s: %v", testNamespace, err)
	}
	// Generates the kube-dns deployment template
	tempArgs := struct{ Namespace, DNSDomain, KubeDNSImage, DNSMasqImage, SidecarImage, KubernetesHost, KubernetesPort string }{
		Namespace:    testNamespace,
		DNSDomain:    defaultDNSDomain,
		KubeDNSImage: defaultKubeDNSImage,
		DNSMasqImage: defaultDNSMasqImage,
		SidecarImage: defaultSideCarImage,
	}
	if host := os.Getenv("KUBERNETES_SERVICE_HOST"); host != "" {
		tempArgs.KubernetesHost = host
	}
	if port := os.Getenv("KUBERNETES_SERVICE_PORT"); port != "" {
		tempArgs.KubernetesPort = port
	}
	dnsDeploymentBytes, err := parseTemplate(kubeDNSDeployment, tempArgs)
	kubeDNSDeployTemplate := &v1beta1.Deployment{}
	if err = kuberuntime.DecodeInto(scheme.Codecs.UniversalDecoder(), dnsDeploymentBytes, kubeDNSDeployTemplate); err != nil {
		t.Fatalf("unable to decode kube-dns deployment in namespace %s:%v", testNamespace, err)
	}

	if !reflect.DeepEqual(kubeDNSDeploy, kubeDNSDeployTemplate) {
		t.Errorf("Created kube-dns deployment in namespace %s has incorrect parameters: %v", testNamespace, kubeDNSDeploy)
	}
}

func TestDeleteDeployment(t *testing.T) {
	testNamespace := "foo"
	// Created a new fake NetworkController.
	controller, _, _, client, err := newNetworkController()
	if err != nil {
		t.Fatalf("Failed start a new fake NetworkController")
	}

	err = controller.createKubeDNSDeployment(testNamespace)
	if err != nil {
		t.Fatalf("Create kube-dns deployment in namespace %v error: %v", testNamespace, err)
	}

	err = controller.deleteDeployment(testNamespace, "kube-dns")
	if err != nil {
		t.Fatalf("Delete kube-dns deployment in namespace %v error: %v", testNamespace, err)
	}
	// test kube-dns deployment not found
	_, err = client.ExtensionsV1beta1().Deployments(testNamespace).Get("kube-dns", apismetav1.GetOptions{})
	if err.Error() != fmt.Errorf("deployments.extensions \"kube-dns\" not found").Error() {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCreateKubeDNSService(t *testing.T) {
	testNamespace := "foo"
	// Created a new fake NetworkController.
	controller, _, _, client, err := newNetworkController()
	if err != nil {
		t.Fatalf("Failed start a new fake NetworkController")
	}

	err = controller.createKubeDNSService(testNamespace)
	if err != nil {
		t.Fatalf("Create kube-dns service in namespace %v error: %v", testNamespace, err)
	}

	kubeDNSSVC, err := client.Core().Services(testNamespace).Get("kube-dns", apismetav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed get kube-dns service in namespace %s: %v", testNamespace, err)
	}
	// Generates the kube-dns service template
	tempArgs := struct{ Namespace string }{
		Namespace: testNamespace,
	}
	dnsServiceBytes, err := parseTemplate(kubeDNSService, tempArgs)
	dnsService := &apiv1.Service{}
	if err = kuberuntime.DecodeInto(scheme.Codecs.UniversalDecoder(), dnsServiceBytes, dnsService); err != nil {
		t.Fatalf("unable to decode kube-dns service in namespace %s: %v", testNamespace, err)
	}

	if !reflect.DeepEqual(kubeDNSSVC, dnsService) {
		t.Errorf("Created kube-dns service in namespace %s has incorrect parameters: %v", testNamespace, kubeDNSSVC)
	}
}

func TestOnAdd(t *testing.T) {
	var controller *NetworkController
	var kubeCRDClient *crdClient.FakeCRDClient
	var osClient *openstack.FakeOSClient
	var client *fake.Clientset
	var err error

	testCases := []struct {
		testName    string
		networkName string
		updateFn    func(networkName string)
		expectedFn  func(networkName string) error
	}{
		{
			testName:    "Add foo1 Network,status active,no related network exist in openstack",
			networkName: "foo1",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// CRD injects fake tenant
				tenant := newTenant(networkName, tenantID)
				kubeCRDClient.SetTenants(tenant)
				// CRD injects fake network
				network := newNetwork(networkName, "")
				kubeCRDClient.SetNetworks(network)
				// openstack injects fake tenant
				osClient.SetTenant(util.BuildNetworkName(networkName, networkName), tenantID)
				// Add network
				controller.onAdd(network)

			},
			expectedFn: func(networkName string) error {
				// test network created
				network, ok := osClient.Networks[util.BuildNetworkName(networkName, networkName)]
				if !ok {
					return fmt.Errorf("expected %s network to be created, got none", networkName)
				} else if network.Name != networkName &&
					network.TenantID != tenantID {
					return fmt.Errorf("the created %s network has incorrect parameters: %v", networkName, network)
				}
				// test network status
				net := kubeCRDClient.Networks[networkName]
				if net.Status.State != crv1.NetworkActive {
					return fmt.Errorf("expected %s network status Active,got %v", networkName, net.Status.State)
				}

				// test kube-dns deployment created
				err = testKubeDNSDeploymentCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service created
				err = testKubeDNSServiceCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:    "Add foo2 Network,status active,network has already created in openstack",
			networkName: "foo2",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// CRD injects fake tenant
				tenant := newTenant(networkName, tenantID)
				kubeCRDClient.SetTenants(tenant)
				// CRD injects fake network
				network := newNetwork(networkName, "")
				kubeCRDClient.SetNetworks(network)
				// openstack injects fake tenant
				osClient.SetTenant(util.BuildNetworkName(networkName, networkName), tenantID)
				// openstack injects fake network
				net := osNetwork(util.BuildNetworkName(networkName, networkName), tenantID, "")
				osClient.SetNetwork(net)
				// Add network
				controller.onAdd(network)

			},
			expectedFn: func(networkName string) error {
				// test network status
				net := kubeCRDClient.Networks[networkName]
				if net.Status.State != crv1.NetworkActive {
					return fmt.Errorf("expected %s network status Active,got %v", networkName, net.Status.State)
				}

				// test kube-dns deployment created
				err = testKubeDNSDeploymentCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service created
				err = testKubeDNSServiceCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:    "Add foo3 Network with spec networkID,status active,network exists in openstack",
			networkName: "foo3",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// CRD injects fake tenant
				tenant := newTenant(networkName, tenantID)
				kubeCRDClient.SetTenants(tenant)
				// CRD injects fake network
				network := newNetwork(networkName, networkID)
				kubeCRDClient.SetNetworks(network)
				// openstack injects fake tenant
				osClient.SetTenant(util.BuildNetworkName(networkName, networkName), tenantID)
				// openstack injects fake network
				net := osNetwork(util.BuildNetworkName(networkName, networkName), tenantID, networkID)
				osClient.SetNetwork(net)
				// Add network
				controller.onAdd(network)

			},
			expectedFn: func(networkName string) error {
				// test network status
				net := kubeCRDClient.Networks[networkName]
				if net.Status.State != crv1.NetworkActive {
					return fmt.Errorf("expected %s network status Active,got %v", networkName, net.Status.State)
				}

				// test kube-dns deployment created
				err = testKubeDNSDeploymentCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service created
				err = testKubeDNSServiceCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:    "Add foo4 Network,status failed,tenant not exist in openstack",
			networkName: "foo4",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// CRD injects fake tenant
				tenant := newTenant(networkName, tenantID)
				kubeCRDClient.SetTenants(tenant)
				// CRD injects fake network
				network := newNetwork(networkName, "")
				kubeCRDClient.SetNetworks(network)
				// Add network
				controller.onAdd(network)

			},
			expectedFn: func(networkName string) error {
				// test network status
				net := kubeCRDClient.Networks[networkName]
				if net.Status.State != crv1.NetworkFailed {
					return fmt.Errorf("expected %s network status Failed,got %v", networkName, net.Status.State)
				}

				// test kube-dns deployment not created
				err = testKubeDNSDeploymentDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service not created
				err = testKubeDNSServiceDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:    "Add foo5 Network with spec networkID,status failed,network not exists in openstack",
			networkName: "foo5",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// CRD injects fake tenant
				tenant := newTenant(networkName, tenantID)
				kubeCRDClient.SetTenants(tenant)
				// CRD injects fake network
				network := newNetwork(networkName, networkID)
				kubeCRDClient.SetNetworks(network)
				// openstack injects fake tenant
				osClient.SetTenant(util.BuildNetworkName(networkName, networkName), tenantID)
				// Add network
				controller.onAdd(network)

			},
			expectedFn: func(networkName string) error {
				// test network status
				net := kubeCRDClient.Networks[networkName]
				if net.Status.State != crv1.NetworkFailed {
					return fmt.Errorf("expected %s network status Failed,got %v", networkName, net.Status.State)
				}

				// test kube-dns deployment not created
				err = testKubeDNSDeploymentDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service not created
				err = testKubeDNSServiceDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:    "Add foo6 Network,status failed,create network failed",
			networkName: "foo6",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// CRD injects fake tenant
				tenant := newTenant(networkName, tenantID)
				kubeCRDClient.SetTenants(tenant)
				// CRD injects fake network
				network := newNetwork(networkName, "")
				kubeCRDClient.SetNetworks(network)
				// openstack injects fake tenant
				osClient.SetTenant(util.BuildNetworkName(networkName, networkName), tenantID)
				// openstack injects createNework error
				osClient.InjectError("CreateNetwork", fmt.Errorf("Failed create network"))
				// Add network
				controller.onAdd(network)

			},
			expectedFn: func(networkName string) error {
				// test network status
				net := kubeCRDClient.Networks[networkName]
				if net.Status.State != crv1.NetworkFailed {
					return fmt.Errorf("expected %s network status Failed,got %v", networkName, net.Status.State)
				}

				// test kube-dns deployment not created
				err = testKubeDNSDeploymentDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service not created
				err = testKubeDNSServiceDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:    "Add foo7 Network,status failed,create network failed",
			networkName: "foo7",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// CRD injects fake tenant
				tenant := newTenant(networkName, tenantID)
				kubeCRDClient.SetTenants(tenant)
				// CRD injects fake network
				network := newNetwork(networkName, "")
				kubeCRDClient.SetNetworks(network)
				// openstack injects fake tenant
				osClient.SetTenant(util.BuildNetworkName(networkName, networkName), tenantID)
				// openstack injects GetNetworkByName error
				osClient.InjectError("GetNetworkByName", fmt.Errorf("Failed get network by name"))
				// Add network
				controller.onAdd(network)

			},
			expectedFn: func(networkName string) error {
				// test network status
				net := kubeCRDClient.Networks[networkName]
				if net.Status.State != crv1.NetworkFailed {
					return fmt.Errorf("expected %s network status Failed,got %v", networkName, net.Status.State)
				}

				// test kube-dns deployment not created
				err = testKubeDNSDeploymentDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service not created
				err = testKubeDNSServiceDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
	}

	for tci, tc := range testCases {
		tc.updateFn(tc.networkName)
		err := tc.expectedFn(tc.networkName)
		if err != nil {
			t.Errorf("Case[%d]: %s %v", tci, tc.testName, err)
		}
	}
}

func TestOnDelete(t *testing.T) {
	var controller *NetworkController
	var kubeCRDClient *crdClient.FakeCRDClient
	var osClient *openstack.FakeOSClient
	var client *fake.Clientset
	var err error

	testCases := []struct {
		testName    string
		networkName string
		updateFn    func(networkName string)
		expectedFn  func(networkName string) error
	}{
		{
			testName:    "Delete foo1 Network with no spec networkID,success",
			networkName: "foo1",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// Create kube-dns deployment
				controller.createKubeDNSDeployment(networkName)
				// Create kube-dns svc
				controller.createKubeDNSService(networkName)
				// openstack injects fake network
				net := osNetwork(util.BuildNetworkName(networkName, networkName), "", "")
				osClient.SetNetwork(net)

				network := newNetwork(networkName, "")
				// Delete network
				controller.onDelete(network)

			},
			expectedFn: func(networkName string) error {
				// test network deleted
				network, ok := osClient.Networks[util.BuildNetworkName(networkName, networkName)]
				if ok {
					return fmt.Errorf("expected %s network to be deleted, got %v", networkName, network)
				}

				// test kube-dns deployment deleted
				err = testKubeDNSDeploymentDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service deleted
				err = testKubeDNSServiceDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:    "Delete foo2 Network with no spec networkID,delete network failed",
			networkName: "foo2",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// Create kube-dns deployment
				controller.createKubeDNSDeployment(networkName)
				// Create kube-dns svc
				controller.createKubeDNSService(networkName)
				// openstack injects fake network
				net := osNetwork(util.BuildNetworkName(networkName, networkName), "", "")
				osClient.SetNetwork(net)
				// openstack injects DeleteNetwork error
				osClient.InjectError("DeleteNetwork", fmt.Errorf("delete network failed"))

				network := newNetwork(networkName, "")
				// Delete network
				controller.onDelete(network)

			},
			expectedFn: func(networkName string) error {
				// test network not be deleted
				_, ok := osClient.Networks[util.BuildNetworkName(networkName, networkName)]
				if !ok {
					return fmt.Errorf("expected %s network not be deleted, got deleted", networkName)
				}

				// test kube-dns deployment deleted
				err = testKubeDNSDeploymentDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service deleted
				err = testKubeDNSServiceDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			testName:    "Delete foo3 Network with spec networkID,the existed network not be deleted",
			networkName: "foo3",
			updateFn: func(networkName string) {

				// Created a new fake NetworkController
				controller, kubeCRDClient, osClient, client, err = newNetworkController()
				if err != nil {
					t.Fatalf("Failed start a new fake NetworkController")
				}
				// Create kube-dns deployment
				controller.createKubeDNSDeployment(networkName)
				// Create kube-dns svc
				controller.createKubeDNSService(networkName)
				// openstack injects fake network
				net := osNetwork(util.BuildNetworkName(networkName, networkName), "", networkID)
				osClient.SetNetwork(net)

				network := newNetwork(networkName, networkID)
				// Delete network
				controller.onDelete(network)

			},
			expectedFn: func(networkName string) error {
				// test network not be deleted
				_, ok := osClient.Networks[util.BuildNetworkName(networkName, networkName)]
				if !ok {
					return fmt.Errorf("expected %s network not be deleted, got deleted", networkName)
				}

				// test kube-dns deployment deleted
				err = testKubeDNSDeploymentDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				// test kube-dns service deleted
				err = testKubeDNSServiceDeletedOrNoCreated(t, client, networkName)
				if err != nil {
					return err
				}
				return nil
			},
		},
	}

	for tci, tc := range testCases {
		tc.updateFn(tc.networkName)
		err := tc.expectedFn(tc.networkName)
		if err != nil {
			t.Errorf("Case[%d]: %s %v", tci, tc.testName, err)
		}
	}
}

func testKubeDNSDeploymentCreated(t *testing.T, client *fake.Clientset, namespace string) error {
	kubeDNSDeploy, err := client.ExtensionsV1beta1().Deployments(namespace).Get("kube-dns", apismetav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed get kube-dns deployment in namespace %s: %v", namespace, err)
	}
	// Generates the kube-dns deployment template
	tempArgs := struct{ Namespace, DNSDomain, KubeDNSImage, DNSMasqImage, SidecarImage, KubernetesHost, KubernetesPort string }{
		Namespace:    namespace,
		DNSDomain:    defaultDNSDomain,
		KubeDNSImage: defaultKubeDNSImage,
		DNSMasqImage: defaultDNSMasqImage,
		SidecarImage: defaultSideCarImage,
	}
	if host := os.Getenv("KUBERNETES_SERVICE_HOST"); host != "" {
		tempArgs.KubernetesHost = host
	}
	if port := os.Getenv("KUBERNETES_SERVICE_PORT"); port != "" {
		tempArgs.KubernetesPort = port
	}
	dnsDeploymentBytes, err := parseTemplate(kubeDNSDeployment, tempArgs)
	kubeDNSDeployTemplate := &v1beta1.Deployment{}
	if err = kuberuntime.DecodeInto(scheme.Codecs.UniversalDecoder(), dnsDeploymentBytes, kubeDNSDeployTemplate); err != nil {
		t.Fatalf("unable to decode kube-dns deployment in namespace %s:%v", namespace, err)
	}

	if !reflect.DeepEqual(kubeDNSDeploy, kubeDNSDeployTemplate) {
		return fmt.Errorf("Created kube-dns deployment in namespace %s has incorrect parameters: %v", namespace, kubeDNSDeploy)
	}
	return nil
}

func testKubeDNSDeploymentDeletedOrNoCreated(t *testing.T, client *fake.Clientset, namespace string) error {
	_, err := client.ExtensionsV1beta1().Deployments(namespace).Get("kube-dns", apismetav1.GetOptions{})

	if err.Error() != fmt.Errorf("deployments.extensions \"kube-dns\" not found").Error() {
		t.Errorf("Unexpected error: %v", err)
	}
	return nil
}

func testKubeDNSServiceCreated(t *testing.T, client *fake.Clientset, namespace string) error {
	kubeDNSSVC, err := client.Core().Services(namespace).Get("kube-dns", apismetav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed get kube-dns service in namespace %s: %v", namespace, err)
	}
	// Generates the kube-dns service template
	tempArgs := struct{ Namespace string }{
		Namespace: namespace,
	}
	dnsServiceBytes, err := parseTemplate(kubeDNSService, tempArgs)
	dnsService := &apiv1.Service{}
	if err = kuberuntime.DecodeInto(scheme.Codecs.UniversalDecoder(), dnsServiceBytes, dnsService); err != nil {
		t.Fatalf("unable to decode kube-dns service in namespace %s: %v", namespace, err)
	}

	if !reflect.DeepEqual(kubeDNSSVC, dnsService) {
		return fmt.Errorf("Created kube-dns service in namespace %s has incorrect parameters: %v", namespace, kubeDNSSVC)
	}
	return nil
}

func testKubeDNSServiceDeletedOrNoCreated(t *testing.T, client *fake.Clientset, namespace string) error {
	_, err := client.Core().Services(namespace).Get("kube-dns", apismetav1.GetOptions{})
	if err.Error() != fmt.Errorf("services \"kube-dns\" not found").Error() {
		return fmt.Errorf("Unexpected error: %v", err)
	}
	return nil
}
