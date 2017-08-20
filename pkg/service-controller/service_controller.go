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
	"reflect"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	informersV1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"
	"github.com/golang/glog"
)

const (
	// Interval of synchronizing service status from apiserver
	serviceSyncPeriod = 30 * time.Second
	resyncPeriod      = 5 * time.Minute

	// How long to wait before retrying the processing of a service change.
	minRetryDelay = 5 * time.Second
	maxRetryDelay = 300 * time.Second

	clientRetryCount       = 5
	clientRetryInterval    = 5 * time.Second
	concurrentServiceSyncs = 1
	retryable              = true
	notRetryable           = false

	doNotRetry = time.Duration(0)
)

type cachedService struct {
	// The cached state of the service
	state *v1.Service
	// Controls error back-off
	lastRetryDelay time.Duration
}

type serviceCache struct {
	mu         sync.Mutex // protects serviceMap
	serviceMap map[string]*cachedService
}

type ServiceController struct {
	cache *serviceCache

	kubeClientset    *kubernetes.Clientset
	osClient         openstack.Interface
	factory          informers.SharedInformerFactory
	serviceInformer  informersV1.ServiceInformer
	endpointInformer informersV1.EndpointsInformer

	// services that need to be synced
	workingQueue workqueue.DelayingInterface
}

// NewServiceController returns a new service controller to keep openstack lbaas resources
// (load balancers) in sync with the registry.
func NewServiceController(kubeClient *kubernetes.Clientset,
	osClient openstack.Interface) (*ServiceController, error) {
	factory := informers.NewSharedInformerFactory(kubeClient, resyncPeriod)
	s := &ServiceController{
		osClient:         osClient,
		factory:          factory,
		kubeClientset:    kubeClient,
		cache:            &serviceCache{serviceMap: make(map[string]*cachedService)},
		workingQueue:     workqueue.NewNamedDelayingQueue("service"),
		serviceInformer:  factory.Core().V1().Services(),
		endpointInformer: factory.Core().V1().Endpoints(),
	}

	s.serviceInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: s.enqueueService,
			UpdateFunc: func(old, cur interface{}) {
				oldSvc, ok1 := old.(*v1.Service)
				curSvc, ok2 := cur.(*v1.Service)
				if ok1 && ok2 && s.needsUpdate(oldSvc, curSvc) {
					s.enqueueService(cur)
				}
			},
			DeleteFunc: s.enqueueService,
		},
		serviceSyncPeriod,
	)

	s.endpointInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: s.enqueueEndpoints,
			UpdateFunc: func(old, cur interface{}) {
				oldEndpoint, ok1 := old.(*v1.Endpoints)
				curEndpoint, ok2 := cur.(*v1.Endpoints)
				if ok1 && ok2 && s.needsUpdateEndpoint(oldEndpoint, curEndpoint) {
					s.enqueueEndpoints(cur)
				}
			},
			DeleteFunc: s.enqueueEndpoints,
		},
		serviceSyncPeriod,
	)

	return s, nil
}

// obj could be an *v1.Service, or a DeletionFinalStateUnknown marker item.
func (s *ServiceController) enqueueService(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		glog.Errorf("Couldn't get key for object %#v: %v", obj, err)
		return
	}
	s.workingQueue.Add(key)
}

// Run starts a background goroutine that watches for changes to services that
// have (or had) LoadBalancers=true and ensures that they have
// load balancers created and deleted appropriately.
// serviceSyncPeriod controls how often we check the cluster's services to
// ensure that the correct load balancers exist.
//
// It's an error to call Run() more than once for a given ServiceController
// object.
func (s *ServiceController) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer s.workingQueue.ShutDown()

	glog.Info("Starting service controller")
	defer glog.Info("Shutting down service controller")

	go s.factory.Start(stopCh)

	if !cache.WaitForCacheSync(stopCh, s.serviceInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to cache services")
	}
	if !cache.WaitForCacheSync(stopCh, s.endpointInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to cache endpoints")
	}

	glog.Infof("Service informer cached")

	for i := 0; i < concurrentServiceSyncs; i++ {
		go wait.Until(s.worker, time.Second, stopCh)
	}

	<-stopCh
	return nil
}

// obj could be an *v1.Endpoint, or a DeletionFinalStateUnknown marker item.
func (s *ServiceController) enqueueEndpoints(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		glog.Errorf("Couldn't get key for object %#v: %v", obj, err)
		return
	}
	s.workingQueue.Add(key)
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (s *ServiceController) worker() {
	for {
		func() {
			key, quit := s.workingQueue.Get()
			if quit {
				return
			}
			defer s.workingQueue.Done(key)
			err := s.syncService(key.(string))
			if err != nil {
				glog.Errorf("Error syncing service: %v", err)
			}
		}()
	}
}

// Returns an error if processing the service update failed, along with a time.Duration
// indicating whether processing should be retried; zero means no-retry; otherwise
// we should retry in that Duration.
func (s *ServiceController) processServiceUpdate(cachedService *cachedService, service *v1.Service,
	key string) (error, time.Duration) {
	// cache the service, we need the info for service deletion
	cachedService.state = service
	err, retry := s.createLoadBalancerIfNeeded(key, service)
	if err != nil {
		message := "Error creating load balancer"
		if retry {
			message += " (will retry): "
		} else {
			message += " (will not retry): "
		}
		message += err.Error()
		glog.V(3).Infof("Create service %q failed: %v, message: %q", buildServiceName(service), err, message)

		return err, cachedService.nextRetryDelay()
	}
	// Always update the cache upon success.
	// NOTE: Since we update the cached service if and only if we successfully
	// processed it, a cached service being nil implies that it hasn't yet
	// been successfully processed.
	s.cache.set(key, cachedService)

	cachedService.resetRetryDelay()
	return nil, doNotRetry
}

// Returns whatever error occurred along with a boolean indicator of whether it should be retried.
func (s *ServiceController) createLoadBalancerIfNeeded(key string, service *v1.Service) (error, bool) {
	// Save the state so we can avoid a write if it doesn't change
	previousState := util.LoadBalancerStatusDeepCopy(&service.Status.LoadBalancer)
	var newState *v1.LoadBalancerStatus
	var err error

	lbName := buildLoadBalancerName(service)
	if !wantsLoadBalancer(service) {
		needDelete := true
		exists, err := s.osClient.LoadBalancerExist(lbName)
		if err != nil {
			return fmt.Errorf("Error getting LB for service %s: %v", key, err), retryable
		}
		if !exists {
			needDelete = false
		}

		if needDelete {
			glog.Infof("Deleting existing load balancer for service %s that no longer needs a load balancer.", key)
			if err := s.osClient.EnsureLoadBalancerDeleted(lbName); err != nil {
				glog.Errorf("EnsureLoadBalancerDeleted %q failed: %v", lbName, err)
				return err, retryable
			}
		}

		newState = &v1.LoadBalancerStatus{}
	} else {
		glog.V(2).Infof("Ensuring LB for service %s", key)

		// The load balancer doesn't exist yet, so create it.
		newState, err = s.createLoadBalancer(service)
		if err != nil {
			return fmt.Errorf("Failed to create load balancer for service %s: %v", key, err), retryable
		}
		glog.V(3).Infof("LoadBalancer %q created", lbName)
	}

	// Write the state if changed
	// TODO: Be careful here ... what if there were other changes to the service?
	if !util.LoadBalancerStatusEqual(previousState, newState) {
		// Make a copy so we don't mutate the shared informer cache
		copy, err := scheme.Scheme.DeepCopy(service)
		if err != nil {
			return err, retryable
		}
		service = copy.(*v1.Service)

		// Update the status on the copy
		service.Status.LoadBalancer = *newState

		if err := s.persistUpdate(service); err != nil {
			return fmt.Errorf("Failed to persist updated status to apiserver, even after retries. Giving up: %v", err), notRetryable
		}
	} else {
		glog.V(2).Infof("Not persisting unchanged LoadBalancerStatus for service %s to kubernetes.", key)
	}

	return nil, notRetryable
}

func (s *ServiceController) persistUpdate(service *v1.Service) error {
	var err error
	for i := 0; i < clientRetryCount; i++ {
		_, err = s.kubeClientset.CoreV1Client.Services(service.Namespace).UpdateStatus(service)
		if err == nil {
			return nil
		}
		// If the object no longer exists, we don't want to recreate it. Just bail
		// out so that we can process the delete, which we should soon be receiving
		// if we haven't already.
		if errors.IsNotFound(err) {
			glog.Infof("Not persisting update to service '%s/%s' that no longer exists: %v",
				service.Namespace, service.Name, err)
			return nil
		}
		// TODO: Try to resolve the conflict if the change was unrelated to load
		// balancer status. For now, just pass it up the stack.
		if errors.IsConflict(err) {
			return fmt.Errorf("Not persisting update to service '%s/%s' that has been changed since we received it: %v",
				service.Namespace, service.Name, err)
		}
		glog.Warningf("Failed to persist updated LoadBalancerStatus to service '%s/%s' after creating its load balancer: %v",
			service.Namespace, service.Name, err)
		time.Sleep(clientRetryInterval)
	}
	return err
}

func (s *ServiceController) createLoadBalancer(service *v1.Service) (*v1.LoadBalancerStatus, error) {
	// Only one protocol and one externalIPs supported per service.
	if len(service.Spec.ExternalIPs) > 1 {
		return nil, fmt.Errorf("multiple floatingips are not supported")
	}
	if len(service.Spec.Ports) > 1 {
		return nil, fmt.Errorf("multiple floatingips are not supported")
	}

	// Only support one network and network's name is same with namespace.
	networkName := util.BuildNetworkName(service.Namespace, service.Namespace)
	network, err := s.osClient.GetNetwork(networkName)
	if err != nil {
		glog.Errorf("Get network by name %q failed: %v", networkName, err)
		return nil, err
	}

	// get endpoints for the service.
	endpoints, err := s.getEndpoints(service)
	if err != nil {
		glog.Errorf("Get endpoints for service %q failed: %v", buildServiceName(service), err)
		return nil, err
	}

	// create the loadbalancer.
	lbName := buildLoadBalancerName(service)
	svcPort := service.Spec.Ports[0]
	externalIP := ""
	if len(service.Spec.ExternalIPs) > 0 {
		externalIP = service.Spec.ExternalIPs[0]
	}
	lb, err := s.osClient.EnsureLoadBalancer(&openstack.LoadBalancer{
		Name:            lbName,
		Endpoints:       endpoints,
		ServicePort:     int(svcPort.Port),
		TenantID:        network.TenantID,
		SubnetID:        network.Subnets[0].Uid,
		Protocol:        string(svcPort.Protocol),
		ExternalIP:      externalIP,
		SessionAffinity: service.Spec.SessionAffinity != v1.ServiceAffinityNone,
	})
	if err != nil {
		glog.Errorf("EnsureLoadBalancer %q failed: %v", lbName, err)
		return nil, err
	}

	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{IP: lb.ExternalIP}},
	}, nil

}

func (s *ServiceController) getEndpoints(service *v1.Service) ([]openstack.Endpoint, error) {
	endpoints, err := s.kubeClientset.CoreV1Client.Endpoints(service.Namespace).Get(service.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	results := make([]openstack.Endpoint, 0)
	for i := range endpoints.Subsets {
		ep := endpoints.Subsets[i]
		for _, ip := range ep.Addresses {
			for _, port := range ep.Ports {
				results = append(results, openstack.Endpoint{
					Address: ip.IP,
					Port:    int(port.Port),
				})
			}
		}
	}

	return results, nil
}

// ListKeys implements the interface required by DeltaFIFO to list the keys we
// already know about.
func (s *serviceCache) ListKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.serviceMap))
	for k := range s.serviceMap {
		keys = append(keys, k)
	}
	return keys
}

// GetByKey returns the value stored in the serviceMap under the given key
func (s *serviceCache) GetByKey(key string) (interface{}, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.serviceMap[key]; ok {
		return v, true, nil
	}
	return nil, false, nil
}

// ListKeys implements the interface required by DeltaFIFO to list the keys we
// already know about.
func (s *serviceCache) allServices() []*v1.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	services := make([]*v1.Service, 0, len(s.serviceMap))
	for _, v := range s.serviceMap {
		services = append(services, v.state)
	}
	return services
}

func (s *serviceCache) get(serviceName string) (*cachedService, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	service, ok := s.serviceMap[serviceName]
	return service, ok
}

func (s *serviceCache) getOrCreate(serviceName string) *cachedService {
	s.mu.Lock()
	defer s.mu.Unlock()
	service, ok := s.serviceMap[serviceName]
	if !ok {
		service = &cachedService{}
		s.serviceMap[serviceName] = service
	}
	return service
}

func (s *serviceCache) set(serviceName string, service *cachedService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serviceMap[serviceName] = service
}

func (s *serviceCache) delete(serviceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.serviceMap, serviceName)
}

func (s *ServiceController) needsUpdateEndpoint(oldEndpoint, newEndpoint *v1.Endpoints) bool {
	if len(oldEndpoint.Subsets) != len(newEndpoint.Subsets) {
		return true
	}

	if !reflect.DeepEqual(oldEndpoint.Subsets, newEndpoint.Subsets) {
		return true
	}

	return false
}

func (s *ServiceController) needsUpdate(oldService *v1.Service, newService *v1.Service) bool {
	if !wantsLoadBalancer(oldService) && !wantsLoadBalancer(newService) {
		return false
	}
	if wantsLoadBalancer(oldService) != wantsLoadBalancer(newService) {
		glog.V(3).Infof("service %q's type changed to LoadBalancer", buildServiceName(newService))
		return true
	}

	if wantsLoadBalancer(newService) && !reflect.DeepEqual(oldService.Spec.LoadBalancerSourceRanges, newService.Spec.LoadBalancerSourceRanges) {
		glog.V(3).Infof("service %q's LoadBalancerSourceRanges changed: %v -> %v", buildServiceName(newService),
			oldService.Spec.LoadBalancerSourceRanges, newService.Spec.LoadBalancerSourceRanges)
		return true
	}

	if !portsEqualForLB(oldService, newService) || oldService.Spec.SessionAffinity != newService.Spec.SessionAffinity {
		return true
	}
	if !loadBalancerIPsAreEqual(oldService, newService) {
		glog.V(3).Infof("service %q's LoadbalancerIP changed: %v -> %v", buildServiceName(newService),
			oldService.Spec.LoadBalancerIP, newService.Spec.LoadBalancerIP)
		return true
	}
	if len(oldService.Spec.ExternalIPs) != len(newService.Spec.ExternalIPs) {
		glog.V(3).Infof("service %q's ExternalIPs changed: %v -> %v", buildServiceName(newService),
			len(oldService.Spec.ExternalIPs), len(newService.Spec.ExternalIPs))
		return true
	}
	for i := range oldService.Spec.ExternalIPs {
		if oldService.Spec.ExternalIPs[i] != newService.Spec.ExternalIPs[i] {
			glog.V(3).Infof("service %q's ExternalIP added: %v", buildServiceName(newService),
				newService.Spec.ExternalIPs[i])
			return true
		}
	}
	if !reflect.DeepEqual(oldService.Annotations, newService.Annotations) {
		return true
	}
	if oldService.UID != newService.UID {
		glog.V(3).Infof("service %q's UID changed: %v -> %v", buildServiceName(newService),
			oldService.UID, newService.UID)
		return true
	}
	if oldService.Spec.ExternalTrafficPolicy != newService.Spec.ExternalTrafficPolicy {
		glog.V(3).Infof("service %q's ExternalTrafficPolicy changed: %v -> %v", buildServiceName(newService),
			oldService.Spec.ExternalTrafficPolicy, newService.Spec.ExternalTrafficPolicy)
		return true
	}

	return false
}

func getPortsForLB(service *v1.Service) ([]*v1.ServicePort, error) {
	var protocol v1.Protocol

	ports := []*v1.ServicePort{}
	for i := range service.Spec.Ports {
		sp := &service.Spec.Ports[i]
		// The check on protocol was removed here.  The cloud provider itself is now responsible for all protocol validation
		ports = append(ports, sp)
		if protocol == "" {
			protocol = sp.Protocol
		} else if protocol != sp.Protocol && wantsLoadBalancer(service) {
			// TODO:  Convert error messages to use event recorder
			return nil, fmt.Errorf("mixed protocol external load balancers are not supported.")
		}
	}
	return ports, nil
}

func portsEqualForLB(x, y *v1.Service) bool {
	xPorts, err := getPortsForLB(x)
	if err != nil {
		return false
	}
	yPorts, err := getPortsForLB(y)
	if err != nil {
		return false
	}
	return portSlicesEqualForLB(xPorts, yPorts)
}

func portSlicesEqualForLB(x, y []*v1.ServicePort) bool {
	if len(x) != len(y) {
		return false
	}

	for i := range x {
		if !portEqualForLB(x[i], y[i]) {
			return false
		}
	}
	return true
}

func portEqualForLB(x, y *v1.ServicePort) bool {
	// TODO: Should we check name?  (In theory, an LB could expose it)
	if x.Name != y.Name {
		return false
	}

	if x.Protocol != y.Protocol {
		return false
	}

	if x.Port != y.Port {
		return false
	}

	// We don't check TargetPort; that is not relevant for load balancing
	// TODO: Should we blank it out?  Or just check it anyway?

	return true
}

func wantsLoadBalancer(service *v1.Service) bool {
	return service.Spec.Type == v1.ServiceTypeLoadBalancer
}

func loadBalancerIPsAreEqual(oldService, newService *v1.Service) bool {
	return oldService.Spec.LoadBalancerIP == newService.Spec.LoadBalancerIP
}

// Computes the next retry, using exponential backoff
// mutex must be held.
func (s *cachedService) nextRetryDelay() time.Duration {
	s.lastRetryDelay = s.lastRetryDelay * 2
	if s.lastRetryDelay < minRetryDelay {
		s.lastRetryDelay = minRetryDelay
	}
	if s.lastRetryDelay > maxRetryDelay {
		s.lastRetryDelay = maxRetryDelay
	}
	return s.lastRetryDelay
}

// Resets the retry exponential backoff.  mutex must be held.
func (s *cachedService) resetRetryDelay() {
	s.lastRetryDelay = time.Duration(0)
}

// syncService will sync the Service with the given key if it has had its expectations fulfilled,
// meaning it did not expect to see any more of its pods created or deleted. This function is not meant to be
// invoked concurrently with the same key.
func (s *ServiceController) syncService(key string) error {
	startTime := time.Now()
	var cachedService *cachedService
	var retryDelay time.Duration
	defer func() {
		glog.V(4).Infof("Finished syncing service %q (%v)", key, time.Now().Sub(startTime))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	// service holds the latest service info from apiserver
	service, err := s.serviceInformer.Lister().Services(namespace).Get(name)
	switch {
	case errors.IsNotFound(err):
		// service absence in store means watcher caught the deletion, ensure LB info is cleaned
		glog.Infof("Service has been deleted %v", key)
		err, retryDelay = s.processServiceDeletion(key)
	case err != nil:
		glog.Infof("Unable to retrieve service %v from store: %v", key, err)
		s.workingQueue.Add(key)
		return err
	default:
		cachedService = s.cache.getOrCreate(key)
		err, retryDelay = s.processServiceUpdate(cachedService, service, key)
	}

	if retryDelay != 0 {
		// Add the failed service back to the queue so we'll retry it.
		glog.Errorf("Failed to process service. Retrying in %s: %v", retryDelay, err)
		go func(obj interface{}, delay time.Duration) {
			// put back the service key to working queue, it is possible that more entries of the service
			// were added into the queue during the delay, but it does not mess as when handling the retry,
			// it always get the last service info from service store
			s.workingQueue.AddAfter(obj, delay)
		}(key, retryDelay)
	} else if err != nil {
		runtime.HandleError(fmt.Errorf("Failed to process service. Not retrying: %v", err))
	}
	return nil
}

// Returns an error if processing the service deletion failed, along with a time.Duration
// indicating whether processing should be retried; zero means no-retry; otherwise
// we should retry after that Duration.
func (s *ServiceController) processServiceDeletion(key string) (error, time.Duration) {
	cachedService, ok := s.cache.get(key)
	if !ok {
		return fmt.Errorf("Service %s not in cache even though the watcher thought it was. Ignoring the deletion.", key), doNotRetry
	}
	service := cachedService.state
	// delete load balancer info only if the service type is LoadBalancer
	if !wantsLoadBalancer(service) {
		return nil, doNotRetry
	}

	lbName := buildLoadBalancerName(service)
	err := s.osClient.EnsureLoadBalancerDeleted(lbName)
	if err != nil {
		glog.Errorf("Error deleting load balancer (will retry): %v", err)
		return err, cachedService.nextRetryDelay()
	}
	glog.V(3).Infof("Loadbalancer %q deleted", lbName)
	s.cache.delete(key)

	cachedService.resetRetryDelay()
	return nil, doNotRetry
}
