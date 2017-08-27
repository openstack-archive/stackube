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

package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.openstack.org/openstack/stackube/pkg/openstack"
	drivertypes "git.openstack.org/openstack/stackube/pkg/openstack/types"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	utiltesting "k8s.io/client-go/util/testing"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/testapi"
)

const (
	LoadBalancerExist         = "LoadBalancerExist"
	EnsureLoadBalancerDeleted = "EnsureLoadBalancerDeleted"
)

func newService(name string, uid types.UID, serviceType v1.ServiceType) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       uid,
			SelfLink:  testapi.Default.SelfLink("services", name),
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 80,
			}},
			ExternalIPs: []string{
				"1.1.1.1",
			},
			Type: serviceType,
		},
	}
}

//Wrap newService so that you dont have to call default argumetns again and again.
func defaultExternalService() *v1.Service {

	return newService("external-balancer", types.UID("123"), v1.ServiceTypeLoadBalancer)

}

func defaultNetwork() *drivertypes.Network {
	subnets := []*drivertypes.Subnet{
		&drivertypes.Subnet{
			Uid: "123",
		},
	}

	return &drivertypes.Network{
		Name:     "kube-default-default",
		TenantID: "123",
		Subnets:  subnets,
	}
}

func makeTestServer(t *testing.T, namespace string) (*httptest.Server, *utiltesting.FakeHandler) {
	fakeEndpointsHandler := utiltesting.FakeHandler{
		StatusCode:   http.StatusOK,
		ResponseBody: runtime.EncodeOrDie(testapi.Default.Codec(), &v1.Endpoints{}),
	}
	mux := http.NewServeMux()
	mux.Handle(testapi.Default.ResourcePath("endpoints/", namespace, ""), &fakeEndpointsHandler)
	mux.Handle(testapi.Default.ResourcePath("services/", namespace, ""), &fakeEndpointsHandler)
	mux.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		t.Errorf("unexpected request: %v", req.RequestURI)
		res.WriteHeader(http.StatusNotFound)
	})
	return httptest.NewServer(mux), &fakeEndpointsHandler
}

func newController() (*ServiceController, *openstack.FakeOSClient, *fake.Clientset) {
	osClient := openstack.NewFake(nil)

	client := fake.NewSimpleClientset()

	controller, _ := NewServiceController(client, osClient)

	return controller, osClient, client
}

func newControllerFakeHTTPServer(url, svcName, namespace string) (*ServiceController, *openstack.FakeOSClient) {
	osClient := openstack.NewFake(nil)

	client := kubernetes.NewForConfigOrDie(&restclient.Config{Host: url, ContentConfig: restclient.ContentConfig{GroupVersion: &api.Registry.GroupOrDie(v1.GroupName).GroupVersion}})

	controller, _ := NewServiceController(client, osClient)

	// Sets fake network.
	osClient.SetNetwork(defaultNetwork())
	// Injects fake endpoint.
	controller.factory.Core().V1().Endpoints().Informer().GetStore().Add(&v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: namespace,
		},
		Subsets: []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{IP: "3.3.3.3"}},
			Ports:     []v1.EndpointPort{{Port: 80}},
		}},
	})

	return controller, osClient
}

func TestServiceTypeNoLoadBalancer(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-external-balancer",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeClusterIP,
		},
	}

	// test load balencer exist or not.
	testCases := map[string]bool{
		"case 1": false,
		"case 2": true,
	}

	lb := &openstack.LoadBalancer{
		Name: buildLoadBalancerName(service),
	}

	for k, lbExist := range testCases {
		controller, osClient, client := newController()
		if lbExist {
			osClient.SetLoadbalancer(lb)
		}

		err, _ := controller.createLoadBalancerIfNeeded("foo/bar", service)
		if err != nil {
			t.Errorf("%v: unexpected error: %v", k, err)
		}
		actions := client.Actions()

		if osClient.GetCalledNames()[0] != LoadBalancerExist {
			t.Errorf("%v: unexpected openstack client calls: %v", k, osClient.GetCalledDetails())
		}

		if lbExist {
			if osClient.GetCalledNames()[1] != EnsureLoadBalancerDeleted {
				t.Errorf("%v: unexpected openstack client calls: %v", k, osClient.GetCalledDetails())
			}
		}

		if len(actions) > 0 {
			t.Errorf("%v: unexpected client actions: %v", k, actions)
		}
	}
}

func TestCreateExternalLoadBalancer(t *testing.T) {
	table := []struct {
		service       *v1.Service
		expectErr     bool
		expectCreated bool
	}{
		{
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					SelfLink:  testapi.Default.SelfLink("services", "svc1"),
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Port: 80,
						},
						{
							Port: 8080,
						},
					},
					Type: v1.ServiceTypeLoadBalancer,
				},
			},
			expectErr:     true,
			expectCreated: false,
		},
		{
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc2",
					Namespace: "default",
					SelfLink:  testapi.Default.SelfLink("services", "svc2"),
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{{
						Port: 80,
					}},
					ExternalIPs: []string{
						"1.1.1.1",
						"2.2.2.2",
					},
					Type: v1.ServiceTypeLoadBalancer,
				},
			},
			expectErr:     true,
			expectCreated: false,
		},
		{
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc3",
					Namespace: "default",
					SelfLink:  testapi.Default.SelfLink("services", "svc3"),
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{{
						Port: 80,
					}},
					ExternalIPs: []string{
						"1.1.1.1",
					},
					Type: v1.ServiceTypeLoadBalancer,
				},
			},
			expectErr:     false,
			expectCreated: true,
		},
	}

	for _, item := range table {
		testServer, endpointsHandler := makeTestServer(t, "default")
		defer testServer.Close()

		// Create a new fake service controller.
		controller, osClient := newControllerFakeHTTPServer(testServer.URL, item.service.Name, item.service.Namespace)

		err, _ := controller.createLoadBalancerIfNeeded("foo/bar", item.service)
		if !item.expectErr && err != nil {
			t.Errorf("unexpected error: %v", err)
		} else if item.expectErr && err == nil {
			t.Errorf("expected error creating %v, got nil", item.service)
		}

		if !item.expectCreated {
			if len(osClient.GetCalledNames()) != 0 {
				t.Errorf("unexpected openstack client calls: %v", osClient.GetCalledDetails())
			}
			endpointsHandler.ValidateRequestCount(t, 0)
		} else {
			var balancer *openstack.LoadBalancer
			for k := range osClient.LoadBalancers {
				if balancer == nil {
					b := osClient.LoadBalancers[k]
					balancer = b
				} else {
					t.Errorf("expected one load balancer to be created, got %v", osClient.LoadBalancers)
					break
				}
			}
			if balancer == nil {
				t.Errorf("expected one load balancer to be created, got none")
			} else if balancer.Name != buildLoadBalancerName(item.service) &&
				balancer.ServicePort != int(item.service.Spec.Ports[0].Port) &&
				balancer.ExternalIP != item.service.Spec.ExternalIPs[0] {
				t.Errorf("created load balancer has incorrect parameters: %v", balancer)
			}
			endpointsHandler.ValidateRequestCount(t, 2)
		}
	}
}

func TestProcessServiceUpdate(t *testing.T) {

	var controller *ServiceController
	var osClient *openstack.FakeOSClient

	//A pair of old and new loadbalancer IP address
	oldLBIP := "192.168.1.1"
	newLBIP := "192.168.1.2"

	testServer, _ := makeTestServer(t, "default")
	defer testServer.Close()

	testCases := []struct {
		testName   string
		key        string
		updateFn   func(*v1.Service) *v1.Service //Manipulate the structure
		svc        *v1.Service
		expectedFn func(*v1.Service, error, time.Duration) error //Error comparision function
	}{
		{
			testName: "If updating a valid service",
			key:      "validKey",
			svc:      defaultExternalService(),
			updateFn: func(svc *v1.Service) *v1.Service {

				// Create a new fake service controller.
				controller, osClient = newControllerFakeHTTPServer(testServer.URL, "external-balancer", "default")
				controller.cache.getOrCreate("validKey")
				return svc

			},
			expectedFn: func(svc *v1.Service, err error, retryDuration time.Duration) error {

				if err != nil {
					return err
				}
				if retryDuration != doNotRetry {
					return fmt.Errorf("retryDuration Expected=%v Obtained=%v", doNotRetry, retryDuration)
				}

				if len(osClient.GetCalledNames()) != 2 {
					t.Errorf("unexpected openstack client calls: %v", osClient.GetCalledDetails())
				}

				return nil
			},
		},
		{
			testName: "If Updating Loadbalancer IP",
			key:      "default/sync-test-name",
			svc:      newService("sync-test-name", types.UID("sync-test-uid"), v1.ServiceTypeLoadBalancer),
			updateFn: func(svc *v1.Service) *v1.Service {

				svc.Spec.LoadBalancerIP = oldLBIP

				keyExpected := svc.GetObjectMeta().GetNamespace() + "/" + svc.GetObjectMeta().GetName()
				controller.enqueueService(svc)
				cachedServiceTest := controller.cache.getOrCreate(keyExpected)
				cachedServiceTest.state = svc
				controller.cache.set(keyExpected, cachedServiceTest)

				keyGot, quit := controller.workingQueue.Get()
				if quit {
					t.Fatalf("get no workingQueue element")
				}
				if keyExpected != keyGot.(string) {
					t.Fatalf("get service key error, expected: %s, got: %s", keyExpected, keyGot.(string))
				}

				copy, err := scheme.Scheme.DeepCopy(svc)
				if err != nil {
					t.Fatalf("copy service error: %v", err)
				}
				newService := copy.(*v1.Service)

				newService.Spec.LoadBalancerIP = newLBIP
				return newService

			},
			expectedFn: func(svc *v1.Service, err error, retryDuration time.Duration) error {

				if err != nil {
					return err
				}
				if retryDuration != doNotRetry {
					return fmt.Errorf("retryDuration Expected=%v Obtained=%v", doNotRetry, retryDuration)
				}

				keyExpected := svc.GetObjectMeta().GetNamespace() + "/" + svc.GetObjectMeta().GetName()

				cachedServiceGot, exist := controller.cache.get(keyExpected)
				if !exist {
					return fmt.Errorf("update service error, workingQueue should contain service: %s", keyExpected)
				}
				if cachedServiceGot.state.Spec.LoadBalancerIP != newLBIP {
					return fmt.Errorf("update LoadBalancerIP error, expected: %s, got: %s", newLBIP, cachedServiceGot.state.Spec.LoadBalancerIP)
				}

				if len(osClient.GetCalledNames()) != 4 {
					t.Errorf("unexpected openstack client calls: %v", osClient.GetCalledDetails())
				}

				return nil
			},
		},
	}

	for _, tc := range testCases {
		newSvc := tc.updateFn(tc.svc)
		svcCache := controller.cache.getOrCreate(tc.key)
		obtErr, retryDuration := controller.processServiceUpdate(svcCache, newSvc, tc.key)
		if err := tc.expectedFn(newSvc, obtErr, retryDuration); err != nil {
			t.Errorf("%v processServiceUpdate() %v", tc.testName, err)
		}
	}

}

func TestSyncService(t *testing.T) {

	var controller *ServiceController
	var osClient *openstack.FakeOSClient

	testServer, _ := makeTestServer(t, "default")
	defer testServer.Close()

	testCases := []struct {
		testName   string
		key        string
		updateFn   func()            //Function to manipulate the controller element to simulate error
		expectedFn func(error) error //Expected function if returns nil then test passed, failed otherwise
	}{
		{
			testName: "if an invalid service name is synced",
			key:      "invalid/key/string",
			updateFn: func() {

				// Create a new fake service controller.
				controller, osClient = newControllerFakeHTTPServer(testServer.URL, "", "")

			},
			expectedFn: func(e error) error {
				//TODO: Expected error is of the format fmt.Errorf("unexpected key format: %q", "invalid/key/string"),
				//TODO: should find a way to test for dependent package errors in such a way that it wont break
				//TODO:	our tests, currently we only test if there is an error.
				//Error should be non-nil
				if e == nil {
					return fmt.Errorf("Expected=unexpected key format: %q, Obtained=nil", "invalid/key/string")
				}
				if len(osClient.GetCalledNames()) != 0 {
					t.Errorf("unexpected openstack client calls: %v", osClient.GetCalledDetails())
				}
				return nil
			},
		},
		/* We cannot open this test case as syncService(key) currently runtime.HandleError(err) and suppresses frequently occurring errors
		{
			testName: "if an invalid service is synced",
			key: "somethingelse",
			updateFn: func() {
				// Create a new fake service controller.
				controller, osClient = newControllerFakeHTTPServer(testServer.URL, "external-balancer", "default")
				srv := controller.cache.getOrCreate("external-balancer")
				srv.state = defaultExternalService()
			},
			expectedErr: fmt.Errorf("Service somethingelse not in cache even though the watcher thought it was. Ignoring the deletion."),
		},
		*/

		//TODO: see if we can add a test for valid but error throwing service, its difficult right now because synCService() currently runtime.HandleError
		{
			testName: "if valid service",
			key:      "external-balancer",
			updateFn: func() {
				testSvc := defaultExternalService()
				// Create a new fake service controller.
				controller, osClient = newControllerFakeHTTPServer(testServer.URL, "external-balancer", "default")
				controller.enqueueService(testSvc)
				svc := controller.cache.getOrCreate("external-balancer")
				svc.state = testSvc
			},
			expectedFn: func(e error) error {
				//error should be nil
				if e != nil {
					return fmt.Errorf("Expected=nil, Obtained=%v", e)
				}
				if osClient.GetCalledDetails()[0].Name != EnsureLoadBalancerDeleted {
					t.Errorf("unexpected openstack client calls: %v", osClient.GetCalledDetails())
				}
				return nil
			},
		},
	}

	for _, tc := range testCases {

		tc.updateFn()
		obtainedErr := controller.syncService(tc.key)

		//expected matches obtained ??.
		if exp := tc.expectedFn(obtainedErr); exp != nil {
			t.Errorf("%v Error:%v", tc.testName, exp)
		}

		//Post processing, the element should not be in the sync queue.
		_, exist := controller.cache.get(tc.key)
		if exist {
			t.Fatalf("%v working Queue should be empty, but contains %s", tc.testName, tc.key)
		}
	}
}

func TestProcessServiceDeletion(t *testing.T) {

	var controller *ServiceController
	var osClient *openstack.FakeOSClient

	testServer, _ := makeTestServer(t, "default")
	defer testServer.Close()
	//Add a global svcKey name
	svcKey := "external-balancer"

	testCases := []struct {
		testName   string
		updateFn   func(*ServiceController)                              //Update function used to manupulate srv and controller values
		expectedFn func(svcErr error, retryDuration time.Duration) error //Function to check if the returned value is expected
	}{
		{
			testName: "If an non-existant service is deleted",
			updateFn: func(controller *ServiceController) {
				//Does not do anything
			},
			expectedFn: func(svcErr error, retryDuration time.Duration) error {

				expectedError := "Service external-balancer not in cache even though the watcher thought it was. Ignoring the deletion."
				if svcErr == nil || svcErr.Error() != expectedError {
					//cannot be nil or Wrong error message
					return fmt.Errorf("Expected=%v Obtained=%v", expectedError, svcErr)
				}

				if retryDuration != doNotRetry {
					//Retry duration should match
					return fmt.Errorf("RetryDuration Expected=%v Obtained=%v", doNotRetry, retryDuration)
				}

				if len(osClient.GetCalledNames()) != 0 {
					t.Errorf("unexpected openstack client calls: %v", osClient.GetCalledDetails())
				}

				return nil
			},
		},
		{
			testName: "If openstack failed to delete the LoadBalancer",
			updateFn: func(controller *ServiceController) {

				svc := controller.cache.getOrCreate(svcKey)
				svc.state = defaultExternalService()
				osClient.InjectError("EnsureLoadBalancerDeleted", fmt.Errorf("Error Deleting the Loadbalancer"))

			},
			expectedFn: func(svcErr error, retryDuration time.Duration) error {

				expectedError := "Error Deleting the Loadbalancer"

				if svcErr == nil || svcErr.Error() != expectedError {
					return fmt.Errorf("Expected=%v Obtained=%v", expectedError, svcErr)
				}

				if retryDuration != minRetryDelay {
					return fmt.Errorf("RetryDuration Expected=%v Obtained=%v", minRetryDelay, retryDuration)
				}

				if len(osClient.GetCalledNames()) != 1 && osClient.GetCalledDetails()[0].Name != EnsureLoadBalancerDeleted {
					t.Errorf("unexpected openstack client calls: %v", osClient.GetCalledDetails())
				}

				return nil
			},
		},
		{
			testName: "If openstack delete loadbalancer successfully",
			updateFn: func(controller *ServiceController) {

				testSvc := defaultExternalService()
				controller.enqueueService(testSvc)
				svc := controller.cache.getOrCreate(svcKey)
				svc.state = testSvc
				controller.cache.set(svcKey, svc)

			},
			expectedFn: func(svcErr error, retryDuration time.Duration) error {

				if svcErr != nil {
					return fmt.Errorf("Expected=nil Obtained=%v", svcErr)
				}

				if retryDuration != doNotRetry {
					//Retry duration should match
					return fmt.Errorf("RetryDuration Expected=%v Obtained=%v", doNotRetry, retryDuration)
				}

				//It should no longer be in the workqueue.
				_, exist := controller.cache.get(svcKey)
				if exist {
					return fmt.Errorf("delete service error, workingQueue should not contain service: %s any more", svcKey)
				}

				if len(osClient.GetCalledNames()) != 0 && osClient.GetCalledDetails()[0].Name != EnsureLoadBalancerDeleted {
					t.Errorf("unexpected openstack client calls: %v", osClient.GetCalledDetails())
				}

				return nil
			},
		},
	}

	for _, tc := range testCases {
		// Create a new fake service controller.
		controller, osClient = newControllerFakeHTTPServer(testServer.URL, "external-balancer", "default")
		tc.updateFn(controller)
		obtainedErr, retryDuration := controller.processServiceDeletion(svcKey)
		if err := tc.expectedFn(obtainedErr, retryDuration); err != nil {
			t.Errorf("%v processServiceDeletion() %v", tc.testName, err)
		}
	}

}

func TestDoesExternalLoadBalancerNeedsUpdate(t *testing.T) {

	var oldSvc, newSvc *v1.Service

	testCases := []struct {
		testName            string //Name of the test case
		updateFn            func() //Function to update the service object
		expectedNeedsUpdate bool   //needsupdate always returns bool

	}{
		{
			testName: "If the service type is changed from LoadBalancer to ClusterIP",
			updateFn: func() {
				oldSvc = defaultExternalService()
				newSvc = defaultExternalService()
				newSvc.Spec.Type = v1.ServiceTypeClusterIP
			},
			expectedNeedsUpdate: true,
		},
		{
			testName: "If the service's LoadBalancerSourceRanges changed",
			updateFn: func() {
				oldSvc = defaultExternalService()
				newSvc = defaultExternalService()
				oldSvc.Spec.LoadBalancerSourceRanges = []string{"old load balancer source range"}
				newSvc.Spec.LoadBalancerSourceRanges = []string{"new load balancer source range"}
			},
			expectedNeedsUpdate: true,
		},
		{
			testName: "If the service's LoadBalancer Port are different",
			updateFn: func() {
				oldSvc = defaultExternalService()
				newSvc = defaultExternalService()
				oldSvc.Spec.Ports = []v1.ServicePort{
					{
						Port: 8000,
					},
				}
				newSvc.Spec.Ports = []v1.ServicePort{
					{
						Port: 8001,
					},
				}

			},
			expectedNeedsUpdate: true,
		},
		{
			testName: "If externel ip counts are different",
			updateFn: func() {
				oldSvc = defaultExternalService()
				newSvc = defaultExternalService()
				oldSvc.Spec.ExternalIPs = []string{"old.IP.1"}
				newSvc.Spec.ExternalIPs = []string{"new.IP.1", "new.IP.2"}
			},
			expectedNeedsUpdate: true,
		},
		{
			testName: "If externel ips are different",
			updateFn: func() {
				oldSvc = defaultExternalService()
				newSvc = defaultExternalService()
				oldSvc.Spec.ExternalIPs = []string{"old.IP.1", "old.IP.2"}
				newSvc.Spec.ExternalIPs = []string{"new.IP.1", "new.IP.2"}
			},
			expectedNeedsUpdate: true,
		},
		{
			testName: "If UID is different",
			updateFn: func() {
				oldSvc = defaultExternalService()
				newSvc = defaultExternalService()
				oldSvc.UID = types.UID("UID old")
				newSvc.UID = types.UID("UID new")
			},
			expectedNeedsUpdate: true,
		},
		{
			testName: "If ExternalTrafficPolicy is different",
			updateFn: func() {
				oldSvc = defaultExternalService()
				newSvc = defaultExternalService()
				newSvc.Spec.ExternalTrafficPolicy = v1.ServiceExternalTrafficPolicyTypeLocal
			},
			expectedNeedsUpdate: true,
		},
	}

	controller, _, _ := newController()
	for _, tc := range testCases {
		tc.updateFn()
		obtainedResult := controller.needsUpdate(oldSvc, newSvc)
		if obtainedResult != tc.expectedNeedsUpdate {
			t.Errorf("%v needsUpdate() should have returned %v but returned %v", tc.testName, tc.expectedNeedsUpdate, obtainedResult)
		}
	}
}

//All the testcases for ServiceCache uses a single cache, these below test cases should be run in order,
//as tc1 (addCache would add elements to the cache)
//and tc2 (delCache would remove element from the cache without it adding automatically)
//Please keep this in mind while adding new test cases.
func TestServiceCache(t *testing.T) {

	//ServiceCache a common service cache for all the test cases
	sc := &serviceCache{serviceMap: make(map[string]*cachedService)}

	testCases := []struct {
		testName     string
		setCacheFn   func()
		checkCacheFn func() error
	}{
		{
			testName: "Add",
			setCacheFn: func() {
				cS := sc.getOrCreate("addTest")
				cS.state = defaultExternalService()
			},
			checkCacheFn: func() error {
				//There must be exactly one element
				if len(sc.serviceMap) != 1 {
					return fmt.Errorf("Expected=1 Obtained=%d", len(sc.serviceMap))
				}
				return nil
			},
		},
		{
			testName: "Del",
			setCacheFn: func() {
				sc.delete("addTest")

			},
			checkCacheFn: func() error {
				//Now it should have no element
				if len(sc.serviceMap) != 0 {
					return fmt.Errorf("Expected=0 Obtained=%d", len(sc.serviceMap))
				}
				return nil
			},
		},
		{
			testName: "Set and Get",
			setCacheFn: func() {
				sc.set("addTest", &cachedService{state: defaultExternalService()})
			},
			checkCacheFn: func() error {
				//Now it should have one element
				Cs, bool := sc.get("addTest")
				if !bool {
					return fmt.Errorf("is Available Expected=true Obtained=%v", bool)
				}
				if Cs == nil {
					return fmt.Errorf("CachedService expected:non-nil Obtained=nil")
				}
				return nil
			},
		},
		{
			testName: "ListKeys",
			setCacheFn: func() {
				//Add one more entry here
				sc.set("addTest1", &cachedService{state: defaultExternalService()})
			},
			checkCacheFn: func() error {
				//It should have two elements
				keys := sc.ListKeys()
				if len(keys) != 2 {
					return fmt.Errorf("Elementes Expected=2 Obtained=%v", len(keys))
				}
				return nil
			},
		},
		{
			testName:   "GetbyKeys",
			setCacheFn: nil, //Nothing to set
			checkCacheFn: func() error {
				//It should have two elements
				svc, isKey, err := sc.GetByKey("addTest")
				if svc == nil || isKey == false || err != nil {
					return fmt.Errorf("Expected(non-nil, true, nil) Obtained(%v,%v,%v)", svc, isKey, err)
				}
				return nil
			},
		},
		{
			testName:   "allServices",
			setCacheFn: nil, //Nothing to set
			checkCacheFn: func() error {
				//It should return two elements
				svcArray := sc.allServices()
				if len(svcArray) != 2 {
					return fmt.Errorf("Expected(2) Obtained(%v)", len(svcArray))
				}
				return nil
			},
		},
	}

	for _, tc := range testCases {
		if tc.setCacheFn != nil {
			tc.setCacheFn()
		}
		if err := tc.checkCacheFn(); err != nil {
			t.Errorf("%v returned %v", tc.testName, err)
		}
	}
}
