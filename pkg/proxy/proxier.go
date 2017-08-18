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
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	informersV1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/util/async"
	utilexec "k8s.io/utils/exec"

	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"
	"github.com/golang/glog"
)

const (
	defaultResyncPeriod = 15 * time.Minute
	minSyncPeriod       = 5 * time.Second
	syncPeriod          = 30 * time.Second
	burstSyncs          = 2
)

type Proxier struct {
	clusterDNS        string
	exec              utilexec.Interface
	kubeClientset     *kubernetes.Clientset
	osClient          *openstack.Client
	factory           informers.SharedInformerFactory
	namespaceInformer informersV1.NamespaceInformer
	serviceInformer   informersV1.ServiceInformer
	endpointInformer  informersV1.EndpointsInformer

	// endpointsChanges and serviceChanges contains all changes to endpoints and
	// services that happened since iptables was synced. For a single object,
	// changes are accumulated, i.e. previous is state from before all of them,
	// current is state after applying all of those.
	endpointsChanges endpointsChangeMap
	serviceChanges   serviceChangeMap
	namespaceChanges namespaceChangeMap

	mu          sync.Mutex // protects the following fields
	initialized int32
	// endpointsSynced and servicesSynced are set to true when corresponding
	// objects are synced after startup. This is used to avoid updating iptables
	// with some partial data after kube-proxy restart.
	endpointsSynced bool
	servicesSynced  bool
	namespaceSynced bool
	serviceMap      proxyServiceMap
	endpointsMap    proxyEndpointsMap
	namespaceMap    map[string]*namespaceInfo
	// service grouping by namespace.
	serviceNSMap map[string]proxyServiceMap
	// governs calls to syncProxyRules
	syncRunner *async.BoundedFrequencyRunner
}

// NewProxier creates a new Proxier.
func NewProxier(kubeConfig, openstackConfig string) (*Proxier, error) {
	// Create OpenStack client from config file.
	osClient, err := openstack.NewClient(openstackConfig, kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could't initialize openstack client: %v", err)
	}

	// Create kubernetes client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := util.NewClusterConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to build clientset: %v", err)
	}

	clusterDNS, err := getClusterDNS(clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster DNS: %v", err)
	}

	factory := informers.NewSharedInformerFactory(clientset, defaultResyncPeriod)
	execer := utilexec.New()
	proxier := &Proxier{
		kubeClientset:    clientset,
		osClient:         osClient,
		factory:          factory,
		clusterDNS:       clusterDNS,
		exec:             execer,
		endpointsChanges: newEndpointsChangeMap(""),
		serviceChanges:   newServiceChangeMap(),
		namespaceChanges: newNamespaceChangeMap(),
		serviceMap:       make(proxyServiceMap),
		endpointsMap:     make(proxyEndpointsMap),
		namespaceMap:     make(map[string]*namespaceInfo),
		serviceNSMap:     make(map[string]proxyServiceMap),
	}
	proxier.syncRunner = async.NewBoundedFrequencyRunner("sync-runner",
		proxier.syncProxyRules, minSyncPeriod, syncPeriod, burstSyncs)
	return proxier, nil
}

func (proxier *Proxier) setInitialized(value bool) {
	var initialized int32
	if value {
		initialized = 1
	}

	atomic.StoreInt32(&proxier.initialized, initialized)
}

func (proxier *Proxier) isInitialized() bool {
	return atomic.LoadInt32(&proxier.initialized) > 0
}

func (p *Proxier) onServiceAdded(obj interface{}) {
	service, ok := obj.(*v1.Service)
	if !ok {
		glog.Errorf("Unexpected object type: %v", obj)
		return
	}

	namespacedName := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}
	if p.serviceChanges.update(&namespacedName, nil, service) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) onServiceUpdated(old, new interface{}) {
	oldService, ok := old.(*v1.Service)
	if !ok {
		glog.Errorf("Unexpected object type: %v", old)
		return
	}
	service, ok := new.(*v1.Service)
	if !ok {
		glog.Errorf("Unexpected object type: %v", new)
		return
	}

	namespacedName := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}
	if p.serviceChanges.update(&namespacedName, oldService, service) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) onServiceDeleted(obj interface{}) {
	service, ok := obj.(*v1.Service)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			glog.Errorf("Unexpected object type: %v", obj)
			return
		}
		if service, ok = tombstone.Obj.(*v1.Service); !ok {
			glog.Errorf("Unexpected object type: %v", obj)
			return
		}
	}

	namespacedName := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}
	if p.serviceChanges.update(&namespacedName, service, nil) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) getRouterForNamespace(namespace string) (string, error) {
	// Only support one network and network's name is same with namespace.
	// TODO: make it general after multi-network is supported.
	networkName := util.BuildNetworkName(namespace, namespace)
	network, err := p.osClient.GetNetwork(networkName)
	if err != nil {
		glog.Errorf("Get network by name %q failed: %v", networkName, err)
		return "", err
	}

	ports, err := p.osClient.ListPorts(network.Uid, "network:router_interface")
	if err != nil {
		glog.Errorf("Get port list for network %q failed: %v", networkName, err)
		return "", err
	}

	if len(ports) == 0 {
		glog.Errorf("Get zero router interface for network %q", networkName)
		return "", fmt.Errorf("no router interface found")
	}

	return ports[0].DeviceID, nil
}

func (p *Proxier) onEndpointsAdded(obj interface{}) {
	endpoints, ok := obj.(*v1.Endpoints)
	if !ok {
		glog.Errorf("Unexpected object type: %v", obj)
		return
	}

	namespacedName := types.NamespacedName{Namespace: endpoints.Namespace, Name: endpoints.Name}
	if p.endpointsChanges.update(&namespacedName, nil, endpoints) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) onEndpointUpdated(old, new interface{}) {
	oldEndpoints, ok := old.(*v1.Endpoints)
	if !ok {
		glog.Errorf("Unexpected object type: %v", old)
		return
	}
	endpoints, ok := new.(*v1.Endpoints)
	if !ok {
		glog.Errorf("Unexpected object type: %v", new)
		return
	}

	namespacedName := types.NamespacedName{Namespace: endpoints.Namespace, Name: endpoints.Name}
	if p.endpointsChanges.update(&namespacedName, oldEndpoints, endpoints) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) onEndpointDeleted(obj interface{}) {
	endpoints, ok := obj.(*v1.Endpoints)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			glog.Errorf("Unexpected object type: %v", obj)
			return
		}
		if endpoints, ok = tombstone.Obj.(*v1.Endpoints); !ok {
			glog.Errorf("Unexpected object type: %v", obj)
			return
		}
	}

	namespacedName := types.NamespacedName{Namespace: endpoints.Namespace, Name: endpoints.Name}
	if p.endpointsChanges.update(&namespacedName, endpoints, nil) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) onNamespaceAdded(obj interface{}) {
	namespace, ok := obj.(*v1.Namespace)
	if !ok {
		glog.Errorf("Unexpected object type: %v", obj)
		return
	}

	if p.namespaceChanges.update(namespace.Name, nil, namespace) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) onNamespaceUpdated(old, new interface{}) {
	oldNamespace, ok := old.(*v1.Namespace)
	if !ok {
		glog.Errorf("Unexpected object type: %v", old)
		return
	}

	namespace, ok := new.(*v1.Namespace)
	if !ok {
		glog.Errorf("Unexpected object type: %v", new)
		return
	}

	if p.namespaceChanges.update(oldNamespace.Name, oldNamespace, namespace) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) onNamespaceDeleted(obj interface{}) {
	namespace, ok := obj.(*v1.Namespace)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			glog.Errorf("Unexpected object type: %v", obj)
			return
		}
		if namespace, ok = tombstone.Obj.(*v1.Namespace); !ok {
			glog.Errorf("Unexpected object type: %v", obj)
			return
		}
	}

	if p.namespaceChanges.update(namespace.Name, namespace, nil) && p.isInitialized() {
		p.syncRunner.Run()
	}
}

func (p *Proxier) RegisterInformers() {
	p.namespaceInformer = p.factory.Core().V1().Namespaces()
	p.namespaceInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    p.onNamespaceAdded,
			UpdateFunc: p.onNamespaceUpdated,
			DeleteFunc: p.onNamespaceDeleted,
		}, time.Minute)

	p.serviceInformer = p.factory.Core().V1().Services()
	p.serviceInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    p.onServiceAdded,
			UpdateFunc: p.onServiceUpdated,
			DeleteFunc: p.onServiceDeleted,
		}, time.Minute)

	p.endpointInformer = p.factory.Core().V1().Endpoints()
	p.endpointInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    p.onEndpointsAdded,
			UpdateFunc: p.onEndpointUpdated,
			DeleteFunc: p.onEndpointDeleted,
		}, time.Minute)
}

func (p *Proxier) StartNamespaceInformer(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, p.namespaceInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to cache namespaces")
	}

	glog.Infof("Namespace informer cached.")

	// Update sync status.
	p.mu.Lock()
	p.namespaceSynced = true
	p.setInitialized(p.servicesSynced && p.endpointsSynced && p.namespaceSynced)
	p.mu.Unlock()

	// Sync unconditionally - this is called once per lifetime.
	p.syncProxyRules()

	return nil
}

func (p *Proxier) StartServiceInformer(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, p.serviceInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to cache services")
	}

	glog.Infof("Services informer cached.")

	// Update sync status.
	p.mu.Lock()
	p.servicesSynced = true
	p.setInitialized(p.servicesSynced && p.endpointsSynced && p.namespaceSynced)
	p.mu.Unlock()

	// Sync unconditionally - this is called once per lifetime.
	p.syncProxyRules()

	return nil
}

func (p *Proxier) StartEndpointInformer(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, p.endpointInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to cache endpoints")
	}

	glog.Infof("Endpoints informer cached.")

	p.mu.Lock()
	p.endpointsSynced = true
	p.setInitialized(p.servicesSynced && p.endpointsSynced && p.namespaceSynced)
	p.mu.Unlock()

	// Sync unconditionally - this is called once per lifetime.
	p.syncProxyRules()

	return nil
}

func (p *Proxier) StartInformerFactory(stopCh <-chan struct{}) error {
	p.factory.Start(stopCh)
	return nil
}

func (p *Proxier) SyncLoop() error {
	p.syncRunner.Loop(wait.NeverStop)
	return nil
}

func (p *Proxier) updateCaches() {
	// Update serviceMap.
	func() {
		p.serviceChanges.lock.Lock()
		defer p.serviceChanges.lock.Unlock()
		for _, change := range p.serviceChanges.items {
			existingPorts := p.serviceMap.merge(change.current)
			p.serviceMap.unmerge(change.previous, existingPorts)
		}

		p.serviceChanges.items = make(map[types.NamespacedName]*serviceChange)
	}()

	// Update services grouping by namespace.
	func() {
		for svc := range p.serviceMap {
			info := p.serviceMap[svc]
			if v, ok := p.serviceNSMap[svc.Namespace]; ok {
				v[svc] = info
			} else {
				p.serviceNSMap[svc.Namespace] = proxyServiceMap{svc: info}
			}
		}
	}()

	// Update endpointsMap.
	func() {
		p.endpointsChanges.lock.Lock()
		defer p.endpointsChanges.lock.Unlock()
		for _, change := range p.endpointsChanges.items {
			p.endpointsMap.unmerge(change.previous)
			p.endpointsMap.merge(change.current)
		}

		p.endpointsChanges.items = make(map[types.NamespacedName]*endpointsChange)
	}()

	// Update namespaceMap and get router for namespaces.
	func() {
		p.namespaceChanges.lock.Lock()
		defer p.namespaceChanges.lock.Unlock()
		for n, change := range p.namespaceChanges.items {
			if change.current == nil {
				delete(p.namespaceMap, n)
			} else {
				if _, ok := p.namespaceMap[n]; !ok {
					p.namespaceMap[n] = change.current
				}

				// get router for the namespace
				if p.namespaceMap[n].router == "" {
					router, err := p.getRouterForNamespace(n)
					if err != nil {
						glog.Warningf("Get router for namespace %q failed: %v. This may be caused by network not ready yet.", n, err)
						continue
					}

					p.namespaceMap[n].router = router
				}
			}
		}

		p.namespaceChanges.items = make(map[string]*namespaceChange)
	}()
}

func (p *Proxier) syncProxyRules() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// don't sync rules until we've received services and endpoints
	if !p.servicesSynced || !p.endpointsSynced || !p.namespaceSynced {
		glog.V(2).Info("Not syncing iptables until services, endpoints and namespaces have been received from master")
		return
	}

	// update local caches.
	p.updateCaches()

	glog.V(3).Infof("Syncing iptables rules")

	// iptablesData contains the iptables rules for netns.
	iptablesData := bytes.NewBuffer(nil)

	// Sync iptables rules for services.
	for namespace := range p.serviceNSMap {
		iptablesData.Reset()

		// Step 1: get namespace info.
		nsInfo, ok := p.namespaceMap[namespace]
		if !ok {
			glog.Errorf("Namespace %q doesn't exist in caches", namespace)
			continue
		}
		glog.V(3).Infof("Syncing iptables for namespace %q: %v", namespace, nsInfo)

		// Step 2: try to get router again since router may be created late after namespaces.
		if nsInfo.router == "" {
			router, err := p.getRouterForNamespace(namespace)
			if err != nil {
				glog.Warningf("Get router for namespace %q failed: %v. This may be caused by network not ready yet.", namespace, err)
				continue
			}
			nsInfo.router = router
		}

		// Step 3: compose iptables chain.
		netns := getRouterNetns(nsInfo.router)
		if !netnsExist(netns) {
			glog.V(3).Infof("Netns %q doesn't exist, omit the services in namespace %q", netns, namespace)
			continue
		}

		ipt := NewIptables(p.exec, netns)
		// ensure chain STACKUBE-PREROUTING created.
		err := ipt.ensureChain()
		if err != nil {
			glog.Errorf("EnsureChain %q in netns %q failed: %v", ChainSKPrerouting, netns, err)
			continue
		}
		// link STACKUBE-PREROUTING chain.
		err = ipt.ensureRule(opAddpendRule, ChainPrerouting, []string{
			"-m", "comment", "--comment", "stackube service portals", "-j", ChainSKPrerouting,
		})
		if err != nil {
			glog.Errorf("Link chain %q in netns %q failed: %v", ChainSKPrerouting, netns, err)
			continue
		}

		// Step 4: flush chain STACKUBE-PREROUTING.
		writeLine(iptablesData, []string{"*nat"}...)
		writeLine(iptablesData, []string{":" + ChainSKPrerouting, "-", "[0:0]"}...)
		writeLine(iptablesData, []string{opFlushChain, ChainSKPrerouting}...)
		writeLine(iptablesData, []string{"COMMIT"}...)

		// Step 5: compose rules for each services.
		glog.V(5).Infof("Syncing iptables for services %v", p.serviceNSMap[namespace])
		writeLine(iptablesData, []string{"*nat"}...)
		for svcName, svcInfo := range p.serviceNSMap[namespace] {
			protocol := strings.ToLower(string(svcInfo.protocol))
			svcNameString := svcInfo.serviceNameString

			// Step 5.1: check service type.
			// Only ClusterIP service is supported. We also handles clusterIP for other typed services, but note that:
			// - NodePort service is not supported since networks are L2 isolated.
			// - LoadBalancer service is handled in service controller.
			if svcInfo.serviceType != v1.ServiceTypeClusterIP {
				glog.V(3).Infof("Only service's clusterIP is handled here, omitting other fields of service %q (type=%q)", svcName.NamespacedName, svcInfo.serviceType)
			}

			// Step 5.2: check endpoints.
			// If the service has no endpoints then do nothing.
			if len(p.endpointsMap[svcName]) == 0 {
				glog.V(3).Infof("No endpoints found for service %q", svcName.NamespacedName)
				continue
			}

			// Step 5.3: Generate the per-endpoint rules.
			// -A STACKUBE-PREROUTING -d 10.108.230.103  -m comment --comment "default/http: cluster IP"
			// -m tcp -p tcp --dport 80 -m statistic --mode random --probability 1.0
			// -j DNAT --to-destination 192.168.1.7:80
			n := len(p.endpointsMap[svcName])
			for i, ep := range p.endpointsMap[svcName] {
				args := []string{
					"-A", ChainSKPrerouting,
					"-m", "comment", "--comment", svcNameString,
					"-m", protocol, "-p", protocol,
					"-d", fmt.Sprintf("%s/32", p.getServiceIP(svcInfo)),
					"--dport", strconv.Itoa(svcInfo.port),
				}

				if i < (n - 1) {
					// Each rule is a probabilistic match.
					args = append(args,
						"-m", "statistic",
						"--mode", "random",
						"--probability", probability(n-i))
				}

				// The final (or only if n == 1) rule is a guaranteed match.
				args = append(args, "-j", "DNAT", "--to-destination", ep.endpoint)
				writeLine(iptablesData, args...)
			}
		}
		writeLine(iptablesData, []string{"COMMIT"}...)

		// Step 6: execute iptables-restore.
		err = ipt.restoreAll(iptablesData.Bytes())
		if err != nil {
			glog.Errorf("Failed to execute iptables-restore: %v", err)
			continue
		}
	}
}

func (p *Proxier) getServiceIP(serviceInfo *serviceInfo) string {
	if serviceInfo.name == "kube-dns" {
		return p.clusterDNS
	}

	return serviceInfo.clusterIP.String()
}

func getClusterDNS(client *kubernetes.Clientset) (string, error) {
	dnssvc, err := client.CoreV1().Services(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err == nil && len(dnssvc.Spec.ClusterIP) > 0 {
		return dnssvc.Spec.ClusterIP, nil
	}

	if apierrors.IsNotFound(err) {
		// get from default namespace.
		k8ssvc, err := client.CoreV1().Services(metav1.NamespaceDefault).Get("kubernetes", metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("couldn't fetch information about the kubernetes service: %v", err)
		}

		// Build an IP by taking the kubernetes service's clusterIP and appending a "0" and checking that it's valid
		dnsIP := net.ParseIP(fmt.Sprintf("%s0", k8ssvc.Spec.ClusterIP))
		if dnsIP == nil {
			return "", fmt.Errorf("could not parse dns ip %q: %v", dnsIP, err)
		}

		return dnsIP.String(), nil
	}

	return "", err
}
