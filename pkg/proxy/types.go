package proxy

import (
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"

	"github.com/golang/glog"

	"git.openstack.org/openstack/stackube/pkg/util"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

// servicePortName carries a namespace + name + portname.  This is the unique
// identfier for a load-balanced service.
type servicePortName struct {
	types.NamespacedName
	Port string
}

func (spn servicePortName) String() string {
	return fmt.Sprintf("%s:%s", spn.NamespacedName.String(), spn.Port)
}

// internal struct for string service information
type serviceInfo struct {
	clusterIP                net.IP
	port                     int
	protocol                 v1.Protocol
	nodePort                 int
	serviceType              v1.ServiceType
	loadBalancerStatus       v1.LoadBalancerStatus
	sessionAffinityType      v1.ServiceAffinity
	stickyMaxAgeMinutes      int
	externalIPs              []string
	loadBalancerSourceRanges []string
	onlyNodeLocalEndpoints   bool
	healthCheckNodePort      int
	// The following fields are computed and stored for performance reasons.
	serviceNameString        string
	servicePortChainName     string
	serviceFirewallChainName string
	serviceLBChainName       string
}

// internal struct for endpoints information
type endpointsInfo struct {
	endpoint string
	isLocal  bool
	// The following fields we lazily compute and store here for performance
	// reasons. If the protocol is the same as you expect it to be, then the
	// chainName can be reused, otherwise it should be recomputed.
	protocol  string
	chainName string
}

type namespaceInfo struct {
	network string
	router  string
}

// Returns just the IP part of the endpoint.
func (e *endpointsInfo) IPPart() string {
	if index := strings.Index(e.endpoint, ":"); index != -1 {
		return e.endpoint[0:index]
	}
	return e.endpoint
}

// Returns the endpoint chain name for a given endpointsInfo.
func (e *endpointsInfo) endpointChain(svcNameString, protocol string) string {
	if e.protocol != protocol {
		e.protocol = protocol
		e.chainName = servicePortEndpointChainName(svcNameString, protocol, e.endpoint)
	}
	return e.chainName
}

func (e *endpointsInfo) String() string {
	return fmt.Sprintf("%v", *e)
}

// returns a new serviceInfo struct
func newServiceInfo(svcPortName servicePortName, port *v1.ServicePort, service *v1.Service) *serviceInfo {
	onlyNodeLocalEndpoints := false
	info := &serviceInfo{
		clusterIP:   net.ParseIP(service.Spec.ClusterIP),
		port:        int(port.Port),
		protocol:    port.Protocol,
		nodePort:    int(port.NodePort),
		serviceType: service.Spec.Type,
		// Deep-copy in case the service instance changes
		loadBalancerStatus:       *util.LoadBalancerStatusDeepCopy(&service.Status.LoadBalancer),
		sessionAffinityType:      service.Spec.SessionAffinity,
		stickyMaxAgeMinutes:      180,
		externalIPs:              make([]string, len(service.Spec.ExternalIPs)),
		loadBalancerSourceRanges: make([]string, len(service.Spec.LoadBalancerSourceRanges)),
		onlyNodeLocalEndpoints:   onlyNodeLocalEndpoints,
	}
	copy(info.loadBalancerSourceRanges, service.Spec.LoadBalancerSourceRanges)
	copy(info.externalIPs, service.Spec.ExternalIPs)

	if needsHealthCheck(service) {
		p := getServiceHealthCheckNodePort(service)
		if p == 0 {
			glog.Errorf("Service %q has no healthcheck nodeport", svcPortName.NamespacedName.String())
		} else {
			info.healthCheckNodePort = int(p)
		}
	}

	// Store the following for performance reasons.
	protocol := strings.ToLower(string(info.protocol))
	info.serviceNameString = svcPortName.String()
	info.servicePortChainName = servicePortChainName(info.serviceNameString, protocol)
	info.serviceFirewallChainName = serviceFirewallChainName(info.serviceNameString, protocol)
	info.serviceLBChainName = serviceLBChainName(info.serviceNameString, protocol)

	return info
}

type endpointsChange struct {
	previous proxyEndpointsMap
	current  proxyEndpointsMap
}

type endpointsChangeMap struct {
	lock     sync.Mutex
	hostname string
	items    map[types.NamespacedName]*endpointsChange
}

type serviceChange struct {
	previous proxyServiceMap
	current  proxyServiceMap
}

type serviceChangeMap struct {
	lock  sync.Mutex
	items map[types.NamespacedName]*serviceChange
}

type namespaceChange struct {
	previous *namespaceInfo
	current  *namespaceInfo
}

type namespaceChangeMap struct {
	lock  sync.Mutex
	items map[string]*namespaceChange
}

type updateEndpointMapResult struct {
	hcEndpoints       map[types.NamespacedName]int
	staleEndpoints    map[endpointServicePair]bool
	staleServiceNames map[servicePortName]bool
}

type updateServiceMapResult struct {
	hcServices    map[types.NamespacedName]uint16
	staleServices sets.String
}

type proxyServiceMap map[servicePortName]*serviceInfo
type proxyEndpointsMap map[servicePortName][]*endpointsInfo

func newEndpointsChangeMap(hostname string) endpointsChangeMap {
	return endpointsChangeMap{
		hostname: hostname,
		items:    make(map[types.NamespacedName]*endpointsChange),
	}
}

func (ecm *endpointsChangeMap) update(namespacedName *types.NamespacedName, previous, current *v1.Endpoints) bool {
	ecm.lock.Lock()
	defer ecm.lock.Unlock()

	change, exists := ecm.items[*namespacedName]
	if !exists {
		change = &endpointsChange{}
		change.previous = endpointsToEndpointsMap(previous, ecm.hostname)
		ecm.items[*namespacedName] = change
	}
	change.current = endpointsToEndpointsMap(current, ecm.hostname)
	if reflect.DeepEqual(change.previous, change.current) {
		delete(ecm.items, *namespacedName)
	}
	return len(ecm.items) > 0
}

func newServiceChangeMap() serviceChangeMap {
	return serviceChangeMap{
		items: make(map[types.NamespacedName]*serviceChange),
	}
}

func (scm *serviceChangeMap) update(namespacedName *types.NamespacedName, previous, current *v1.Service) bool {
	scm.lock.Lock()
	defer scm.lock.Unlock()

	change, exists := scm.items[*namespacedName]
	if !exists {
		change = &serviceChange{}
		change.previous = serviceToServiceMap(previous)
		scm.items[*namespacedName] = change
	}
	change.current = serviceToServiceMap(current)
	if reflect.DeepEqual(change.previous, change.current) {
		delete(scm.items, *namespacedName)
	}
	return len(scm.items) > 0
}

func (sm *proxyServiceMap) merge(other proxyServiceMap) sets.String {
	existingPorts := sets.NewString()
	for svcPortName, info := range other {
		existingPorts.Insert(svcPortName.Port)
		_, exists := (*sm)[svcPortName]
		if !exists {
			glog.V(1).Infof("Adding new service port %q at %s:%d/%s", svcPortName, info.clusterIP, info.port, info.protocol)
		} else {
			glog.V(1).Infof("Updating existing service port %q at %s:%d/%s", svcPortName, info.clusterIP, info.port, info.protocol)
		}
		(*sm)[svcPortName] = info
	}
	return existingPorts
}

func (sm *proxyServiceMap) unmerge(other proxyServiceMap, existingPorts sets.String) {
	for svcPortName := range other {
		if existingPorts.Has(svcPortName.Port) {
			continue
		}
		_, exists := (*sm)[svcPortName]
		if exists {
			glog.V(1).Infof("Removing service port %q", svcPortName)
			//if info.protocol == v1.ProtocolUDP {
			//	staleServices.Insert(info.clusterIP.String())
			//}
			delete(*sm, svcPortName)
		} else {
			glog.Errorf("Service port %q removed, but doesn't exists", svcPortName)
		}
	}
}

func (em proxyEndpointsMap) merge(other proxyEndpointsMap) {
	for svcPortName := range other {
		em[svcPortName] = other[svcPortName]
	}
}

func (em proxyEndpointsMap) unmerge(other proxyEndpointsMap) {
	for svcPortName := range other {
		delete(em, svcPortName)
	}
}

func newNamespaceChangeMap() namespaceChangeMap {
	return namespaceChangeMap{
		items: make(map[string]*namespaceChange),
	}
}

func (ncm *namespaceChangeMap) update(name string, previous, current *v1.Namespace) bool {
	ncm.lock.Lock()
	defer ncm.lock.Unlock()

	change, exists := ncm.items[name]
	if !exists {
		change = &namespaceChange{}
		change.previous = &namespaceInfo{network: name}
		ncm.items[name] = change
	}
	if current != nil {
		change.current = &namespaceInfo{network: name, router: change.previous.router}
	}

	if previous != nil && current != nil {
		delete(ncm.items, name)
	}
	return len(ncm.items) > 0
}
