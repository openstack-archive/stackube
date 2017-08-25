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

package proxy

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"

	crdClient "git.openstack.org/openstack/stackube/pkg/kubecrd"
	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/util/async"
)

func newFakeServiceInfo(serviceName string, ip net.IP) *serviceInfo {
	return &serviceInfo{
		name:      serviceName,
		clusterIP: ip,
	}
}

func Test_getServiceIP(t *testing.T) {
	fp := NewFakeProxier(nil, nil)

	testCases := []struct {
		serviceInfo *serviceInfo
		expected    string
	}{{
		// Case[0]: kube-dns service.
		serviceInfo: newFakeServiceInfo("kube-dns", net.IPv4(1, 2, 3, 4)),
		expected:    testclusterDNS,
	}, {
		// Case[1]: other service.
		serviceInfo: newFakeServiceInfo("test", net.IPv4(1, 2, 3, 4)),
		expected:    "1.2.3.4",
	},
	}

	for tci, tc := range testCases {
		// outputs
		clusterIP := fp.getServiceIP(tc.serviceInfo)

		if clusterIP != tc.expected {
			t.Errorf("Case[%d] expected %#v, got %#v", tci, tc.expected, clusterIP)
		}
	}
}

const testclusterDNS = "10.20.30.40"

func NewFakeProxier(ipt iptablesInterface, osClient openstack.Interface) *Proxier {
	p := &Proxier{
		clusterDNS:       testclusterDNS,
		osClient:         osClient,
		iptables:         ipt,
		endpointsChanges: newEndpointsChangeMap(""),
		serviceChanges:   newServiceChangeMap(),
		namespaceChanges: newNamespaceChangeMap(),
		serviceMap:       make(proxyServiceMap),
		endpointsMap:     make(proxyEndpointsMap),
		namespaceMap:     make(map[string]*namespaceInfo),
		serviceNSMap:     make(map[string]proxyServiceMap),
	}

	p.syncRunner = async.NewBoundedFrequencyRunner("test-sync-runner", p.syncProxyRules, 0, time.Minute, 1)
	return p
}

func hasDNAT(rules []Rule, endpoint string) bool {
	for _, r := range rules {
		if r[ToDest] == endpoint {
			return true
		}
	}
	return false
}

func makeTestService(namespace, name string, svcFunc func(*v1.Service)) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{},
		},
		Spec:   v1.ServiceSpec{},
		Status: v1.ServiceStatus{},
	}
	svcFunc(svc)
	return svc
}

func makeServiceMap(proxier *Proxier, allServices ...*v1.Service) {
	for i := range allServices {
		proxier.onServiceAdded(allServices[i])
	}

	proxier.mu.Lock()
	defer proxier.mu.Unlock()
	proxier.servicesSynced = true
}

func makeTestEndpoints(namespace, name string, eptFunc func(*v1.Endpoints)) *v1.Endpoints {
	ept := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	eptFunc(ept)
	return ept
}

func makeEndpointsMap(proxier *Proxier, allEndpoints ...*v1.Endpoints) {
	for i := range allEndpoints {
		proxier.onEndpointsAdded(allEndpoints[i])
	}

	proxier.mu.Lock()
	defer proxier.mu.Unlock()
	proxier.endpointsSynced = true
}

func makeTestNamespace(name string) *v1.Namespace {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: map[string]string{},
		},
		Spec:   v1.NamespaceSpec{},
		Status: v1.NamespaceStatus{},
	}
	return ns
}

func makeNamespaceMap(proxier *Proxier, allNamespaces ...*v1.Namespace) {
	for i := range allNamespaces {
		proxier.onNamespaceAdded(allNamespaces[i])
	}

	proxier.mu.Lock()
	defer proxier.mu.Unlock()
	proxier.namespaceSynced = true
}

func makeNSN(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

func makeServicePortName(ns, name, port string) servicePortName {
	return servicePortName{
		NamespacedName: makeNSN(ns, name),
		Port:           port,
	}
}

func errorf(msg string, rules []Rule, t *testing.T) {
	for _, r := range rules {
		t.Logf("%q", r)
	}
	t.Errorf("%v", msg)
}

func TestClusterNoEndpoint(t *testing.T) {
	testNamespace := "test"
	svcIP := "1.2.3.4"
	svcPort := 80
	svcPortName := servicePortName{
		NamespacedName: makeNSN(testNamespace, "svc1"),
		Port:           "80",
	}

	// Creates fake iptables.
	ipt := NewFake()
	// Creates fake CRD client.
	crdClient, err := crdClient.NewFake()
	if err != nil {
		t.Fatal("Failed init fake CRD client")
	}
	// Create a fake openstack client.
	osClient := openstack.NewFake(crdClient)
	// Injects fake network.
	networkName := util.BuildNetworkName(testNamespace, testNamespace)
	osClient.SetNetwork(networkName, "123")
	// Injects fake port.
	osClient.SetPort("123", "network:router_interface", "123")
	// Creates a new fake proxier.
	fp := NewFakeProxier(ipt, osClient)

	makeServiceMap(fp,
		makeTestService(svcPortName.Namespace, svcPortName.Name, func(svc *v1.Service) {
			svc.Spec.ClusterIP = svcIP
			svc.Spec.Type = v1.ServiceTypeNodePort
			svc.Spec.Ports = []v1.ServicePort{{
				Name:     svcPortName.Port,
				Port:     int32(svcPort),
				Protocol: v1.ProtocolTCP,
			}}
		}),
	)

	makeEndpointsMap(fp)

	makeNamespaceMap(fp, makeTestNamespace(svcPortName.Namespace))

	fp.syncProxyRules()

	stackubeRules := ipt.GetRules(ChainSKPrerouting, "qrouter-123")
	if len(stackubeRules) != 0 {
		errorf(fmt.Sprintf("Unexpected rule for chain %v without endpoints in namespace %v", ChainSKPrerouting, svcPortName.Namespace), stackubeRules, t)
	}
}

func noClusterIPType(t *testing.T, svcType v1.ServiceType) []Rule {
	testNamespace := "test"
	svcIP := "1.2.3.4"
	svcPort := 80
	svcPortName := servicePortName{
		NamespacedName: makeNSN(testNamespace, "svc1"),
		Port:           "80",
	}

	// Creates fake iptables.
	ipt := NewFake()
	// Creates fake CRD client.
	crdClient, err := crdClient.NewFake()
	if err != nil {
		t.Fatal("Failed init fake CRD client")
	}
	// Create a fake openstack client.
	osClient := openstack.NewFake(crdClient)
	// Injects fake network.
	networkName := util.BuildNetworkName(testNamespace, testNamespace)
	osClient.SetNetwork(networkName, "123")
	// Injects fake port.
	osClient.SetPort("123", "network:router_interface", "123")
	// Creates a new fake proxier.
	fp := NewFakeProxier(ipt, osClient)

	makeServiceMap(fp,
		makeTestService(svcPortName.Namespace, svcPortName.Namespace, func(svc *v1.Service) {
			svc.Spec.ClusterIP = svcIP
			svc.Spec.Type = svcType
			svc.Spec.Ports = []v1.ServicePort{{
				Name:     svcPortName.Port,
				Port:     int32(svcPort),
				Protocol: v1.ProtocolTCP,
			}}
		}),
	)

	makeEndpointsMap(fp)

	makeNamespaceMap(fp, makeTestNamespace(svcPortName.Namespace))

	fp.syncProxyRules()

	stackubeRules := ipt.GetRules(ChainSKPrerouting, "qrouter-123")
	return stackubeRules
}

func TestNoClusterIPType(t *testing.T) {
	testCases := map[string]v1.ServiceType{
		"case 1": v1.ServiceTypeNodePort,
		"case 2": v1.ServiceTypeLoadBalancer,
		"case 3": v1.ServiceTypeExternalName,
	}

	for k, tc := range testCases {
		got := noClusterIPType(t, tc)
		if len(got) != 0 {
			errorf(fmt.Sprintf("%v: unexpected rule for chain %v without ClusterIP service type", k, ChainSKPrerouting), got, t)
		}
	}
}

func TestClusterIPEndpointsJump(t *testing.T) {
	testNamespace := "test"
	svcIP := "1.2.3.4"
	svcPort := 80
	svcPortName := servicePortName{
		NamespacedName: makeNSN(testNamespace, "svc1"),
		Port:           "80",
	}

	// Creates fake iptables.
	ipt := NewFake()
	// Creates fake CRD client.
	crdClient, err := crdClient.NewFake()
	if err != nil {
		t.Fatal("Failed init fake CRD client")
	}
	// Create a fake openstack client.
	osClient := openstack.NewFake(crdClient)
	// Injects fake network.
	networkName := util.BuildNetworkName(testNamespace, testNamespace)
	osClient.SetNetwork(networkName, "123")
	// Injects fake port.
	osClient.SetPort("123", "network:router_interface", "123")
	// Creates a new fake proxier.
	fp := NewFakeProxier(ipt, osClient)

	makeServiceMap(fp,
		makeTestService(svcPortName.Namespace, svcPortName.Name, func(svc *v1.Service) {
			svc.Spec.ClusterIP = svcIP
			svc.Spec.Ports = []v1.ServicePort{{
				Name:     svcPortName.Port,
				Port:     int32(svcPort),
				Protocol: v1.ProtocolTCP,
			}}
		}),
	)

	epIP := "192.168.0.1"
	makeEndpointsMap(fp,
		makeTestEndpoints(svcPortName.Namespace, svcPortName.Name, func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: epIP,
				}},
				Ports: []v1.EndpointPort{{
					Name: svcPortName.Port,
					Port: int32(svcPort),
				}},
			}}
		}),
	)

	makeNamespaceMap(fp, makeTestNamespace(svcPortName.Namespace))

	fp.syncProxyRules()

	epStr := fmt.Sprintf("%s:%d", epIP, svcPort)

	stackubeRules := ipt.GetRules(string(ChainSKPrerouting), "qrouter-123")
	if len(stackubeRules) == 0 {
		errorf(fmt.Sprintf("Unexpected rule for chain %v with endpoints in namespace %v", ChainSKPrerouting, svcPortName.Namespace), stackubeRules, t)
	}
	if !hasDNAT(stackubeRules, epStr) {
		errorf(fmt.Sprintf("Chain %v lacks DNAT to %v", ChainSKPrerouting, epStr), stackubeRules, t)
	}
}

func TestMultiNamespacesService(t *testing.T) {
	ns1 := "ns1"
	svcIP1 := "1.2.3.4"
	svcPort1 := 80
	svcPortName1 := servicePortName{
		NamespacedName: makeNSN(ns1, "svc1"),
		Port:           "80",
	}

	ns2 := "ns2"
	svcIP2 := "1.2.3.5"
	svcPort2 := 8080
	svcPortName2 := servicePortName{
		NamespacedName: makeNSN(ns2, "svc1"),
		Port:           "8080",
	}

	// Creates fake iptables.
	ipt := NewFake()
	// Creates fake CRD client.
	crdClient, err := crdClient.NewFake()
	if err != nil {
		t.Fatal("Failed init fake CRD client")
	}
	// Create a fake openstack client.
	osClient := openstack.NewFake(crdClient)
	// Injects fake network.
	networkName1 := util.BuildNetworkName(ns1, ns1)
	osClient.SetNetwork(networkName1, "123")
	networkName2 := util.BuildNetworkName(ns2, ns2)
	osClient.SetNetwork(networkName2, "456")
	// Injects fake port.
	osClient.SetPort("123", "network:router_interface", "123")
	osClient.SetPort("456", "network:router_interface", "456")
	// Creates a new fake proxier.
	fp := NewFakeProxier(ipt, osClient)

	makeServiceMap(fp,
		makeTestService(svcPortName1.Namespace, svcPortName1.Name, func(svc *v1.Service) {
			svc.Spec.ClusterIP = svcIP1
			svc.Spec.Ports = []v1.ServicePort{{
				Name:     svcPortName1.Port,
				Port:     int32(svcPort1),
				Protocol: v1.ProtocolTCP,
			}}
		}),
		makeTestService(svcPortName2.Namespace, svcPortName2.Name, func(svc *v1.Service) {
			svc.Spec.ClusterIP = svcIP2
			svc.Spec.Ports = []v1.ServicePort{{
				Name:     svcPortName2.Port,
				Port:     int32(svcPort2),
				Protocol: v1.ProtocolTCP,
			}}
		}),
	)

	epIP1 := "192.168.0.1"
	epIP2 := "192.168.1.1"
	makeEndpointsMap(fp,
		makeTestEndpoints(svcPortName1.Namespace, svcPortName1.Name, func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: epIP1,
				}},
				Ports: []v1.EndpointPort{{
					Name: svcPortName1.Port,
					Port: int32(svcPort1),
				}},
			}}
		}),
		makeTestEndpoints(svcPortName2.Namespace, svcPortName2.Name, func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{
					IP: epIP2,
				}},
				Ports: []v1.EndpointPort{{
					Name: svcPortName2.Port,
					Port: int32(svcPort2),
				}},
			}}
		}),
	)

	makeNamespaceMap(fp,
		makeTestNamespace(svcPortName1.Namespace),
		makeTestNamespace(svcPortName2.Namespace),
	)

	fp.syncProxyRules()

	epStr1 := fmt.Sprintf("%s:%d", epIP1, svcPort1)

	ns1Rules := ipt.GetRules(string(ChainSKPrerouting), "qrouter-123")
	if len(ns1Rules) == 0 {
		errorf(fmt.Sprintf("Unexpected rule for chain %v with endpoints in namespace %v", ChainSKPrerouting, svcPortName1.Namespace), ns1Rules, t)
	}
	if !hasDNAT(ns1Rules, epStr1) {
		errorf(fmt.Sprintf("Chain %v lacks DNAT to %v", ChainSKPrerouting, epStr1), ns1Rules, t)
	}

	epStr2 := fmt.Sprintf("%s:%d", epIP2, svcPort2)
	ns2Rules := ipt.GetRules(string(ChainSKPrerouting), "qrouter-456")
	if len(ns2Rules) == 0 {
		errorf(fmt.Sprintf("Unexpected rule for chain %v with endpoints in namespace %v", ChainSKPrerouting, svcPortName2.Namespace), ns2Rules, t)
	}
	if !hasDNAT(ns2Rules, epStr2) {
		errorf(fmt.Sprintf("Chain %v lacks DNAT to %v", ChainSKPrerouting, epStr2), ns2Rules, t)
	}
}

// This is a coarse test, but it offers some modicum of confidence as the code is evolved.
func Test_endpointsToEndpointsMap(t *testing.T) {
	testCases := []struct {
		newEndpoints *v1.Endpoints
		expected     map[servicePortName][]*endpointsInfo
	}{{
		// Case[0]: nothing
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {}),
		expected:     map[servicePortName][]*endpointsInfo{},
	}, {
		// Case[1]: no changes, unnamed port
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{
				{
					Addresses: []v1.EndpointAddress{{
						IP: "1.1.1.1",
					}},
					Ports: []v1.EndpointPort{{
						Name: "",
						Port: 11,
					}},
				},
			}
		}),
		expected: map[servicePortName][]*endpointsInfo{
			makeServicePortName("ns1", "ep1", ""): {
				{endpoint: "1.1.1.1:11", isLocal: false},
			},
		},
	}, {
		// Case[2]: no changes, named port
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{
				{
					Addresses: []v1.EndpointAddress{{
						IP: "1.1.1.1",
					}},
					Ports: []v1.EndpointPort{{
						Name: "port",
						Port: 11,
					}},
				},
			}
		}),
		expected: map[servicePortName][]*endpointsInfo{
			makeServicePortName("ns1", "ep1", "port"): {
				{endpoint: "1.1.1.1:11", isLocal: false},
			},
		},
	}, {
		// Case[3]: new port
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{
				{
					Addresses: []v1.EndpointAddress{{
						IP: "1.1.1.1",
					}},
					Ports: []v1.EndpointPort{{
						Port: 11,
					}},
				},
			}
		}),
		expected: map[servicePortName][]*endpointsInfo{
			makeServicePortName("ns1", "ep1", ""): {
				{endpoint: "1.1.1.1:11", isLocal: false},
			},
		},
	}, {
		// Case[4]: remove port
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {}),
		expected:     map[servicePortName][]*endpointsInfo{},
	}, {
		// Case[5]: new IP and port
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{
				{
					Addresses: []v1.EndpointAddress{{
						IP: "1.1.1.1",
					}, {
						IP: "2.2.2.2",
					}},
					Ports: []v1.EndpointPort{{
						Name: "p1",
						Port: 11,
					}, {
						Name: "p2",
						Port: 22,
					}},
				},
			}
		}),
		expected: map[servicePortName][]*endpointsInfo{
			makeServicePortName("ns1", "ep1", "p1"): {
				{endpoint: "1.1.1.1:11", isLocal: false},
				{endpoint: "2.2.2.2:11", isLocal: false},
			},
			makeServicePortName("ns1", "ep1", "p2"): {
				{endpoint: "1.1.1.1:22", isLocal: false},
				{endpoint: "2.2.2.2:22", isLocal: false},
			},
		},
	}, {
		// Case[6]: remove IP and port
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{
				{
					Addresses: []v1.EndpointAddress{{
						IP: "1.1.1.1",
					}},
					Ports: []v1.EndpointPort{{
						Name: "p1",
						Port: 11,
					}},
				},
			}
		}),
		expected: map[servicePortName][]*endpointsInfo{
			makeServicePortName("ns1", "ep1", "p1"): {
				{endpoint: "1.1.1.1:11", isLocal: false},
			},
		},
	}, {
		// Case[7]: rename port
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{
				{
					Addresses: []v1.EndpointAddress{{
						IP: "1.1.1.1",
					}},
					Ports: []v1.EndpointPort{{
						Name: "p2",
						Port: 11,
					}},
				},
			}
		}),
		expected: map[servicePortName][]*endpointsInfo{
			makeServicePortName("ns1", "ep1", "p2"): {
				{endpoint: "1.1.1.1:11", isLocal: false},
			},
		},
	}, {
		// Case[8]: renumber port
		newEndpoints: makeTestEndpoints("ns1", "ep1", func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{
				{
					Addresses: []v1.EndpointAddress{{
						IP: "1.1.1.1",
					}},
					Ports: []v1.EndpointPort{{
						Name: "p1",
						Port: 22,
					}},
				},
			}
		}),
		expected: map[servicePortName][]*endpointsInfo{
			makeServicePortName("ns1", "ep1", "p1"): {
				{endpoint: "1.1.1.1:22", isLocal: false},
			},
		},
	}}

	for tci, tc := range testCases {
		// outputs
		newEndpoints := endpointsToEndpointsMap(tc.newEndpoints, "host")

		if len(newEndpoints) != len(tc.expected) {
			t.Errorf("[%d] expected %d new, got %d: %v", tci, len(tc.expected), len(newEndpoints), spew.Sdump(newEndpoints))
		}
		for x := range tc.expected {
			if len(newEndpoints[x]) != len(tc.expected[x]) {
				t.Errorf("[%d] expected %d endpoints for %v, got %d", tci, len(tc.expected[x]), x, len(newEndpoints[x]))
			} else {
				for i := range newEndpoints[x] {
					if *(newEndpoints[x][i]) != *(tc.expected[x][i]) {
						t.Errorf("[%d] expected new[%v][%d] to be %v, got %v", tci, x, i, tc.expected[x][i], *(newEndpoints[x][i]))
					}
				}
			}
		}
	}
}
