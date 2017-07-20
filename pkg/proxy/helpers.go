package proxy

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Translates single Endpoints object to proxyEndpointsMap.
// This function is used for incremental updated of endpointsMap.
//
// NOTE: endpoints object should NOT be modified.
func endpointsToEndpointsMap(endpoints *v1.Endpoints, hostname string) proxyEndpointsMap {
	if endpoints == nil {
		return nil
	}

	endpointsMap := make(proxyEndpointsMap)
	// We need to build a map of portname -> all ip:ports for that
	// portname.  Explode Endpoints.Subsets[*] into this structure.
	for i := range endpoints.Subsets {
		ss := &endpoints.Subsets[i]
		for i := range ss.Ports {
			port := &ss.Ports[i]
			if port.Port == 0 {
				glog.Warningf("ignoring invalid endpoint port %s", port.Name)
				continue
			}
			svcPortName := servicePortName{
				NamespacedName: types.NamespacedName{Namespace: endpoints.Namespace, Name: endpoints.Name},
				Port:           port.Name,
			}
			for i := range ss.Addresses {
				addr := &ss.Addresses[i]
				if addr.IP == "" {
					glog.Warningf("ignoring invalid endpoint port %s with empty host", port.Name)
					continue
				}
				epInfo := &endpointsInfo{
					endpoint: net.JoinHostPort(addr.IP, strconv.Itoa(int(port.Port))),
					isLocal:  addr.NodeName != nil && *addr.NodeName == hostname,
				}
				endpointsMap[svcPortName] = append(endpointsMap[svcPortName], epInfo)
			}
			if glog.V(3) {
				newEPList := []string{}
				for _, ep := range endpointsMap[svcPortName] {
					newEPList = append(newEPList, ep.endpoint)
				}
				glog.Infof("Setting endpoints for %q to %+v", svcPortName, newEPList)
			}
		}
	}
	return endpointsMap
}

// Translates single Service object to proxyServiceMap.
//
// NOTE: service object should NOT be modified.
func serviceToServiceMap(service *v1.Service) proxyServiceMap {
	if service == nil {
		return nil
	}
	svcName := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}
	if shouldSkipService(svcName, service) {
		return nil
	}

	serviceMap := make(proxyServiceMap)
	for i := range service.Spec.Ports {
		servicePort := &service.Spec.Ports[i]
		svcPortName := servicePortName{NamespacedName: svcName, Port: servicePort.Name}
		serviceMap[svcPortName] = newServiceInfo(svcPortName, servicePort, service)
	}
	return serviceMap
}

// portProtoHash takes the servicePortName and protocol for a service
// returns the associated 16 character hash. This is computed by hashing (sha256)
// then encoding to base32 and truncating to 16 chars. We do this because IPTables
// Chain Names must be <= 28 chars long, and the longer they are the harder they are to read.
func portProtoHash(servicePortName string, protocol string) string {
	hash := sha256.Sum256([]byte(servicePortName + protocol))
	encoded := base32.StdEncoding.EncodeToString(hash[:])
	return encoded[:16]
}

// servicePortChainName takes the servicePortName for a service and
// returns the associated iptables chain.  This is computed by hashing (sha256)
// then encoding to base32 and truncating with the prefix "KUBE-SVC-".
func servicePortChainName(servicePortName string, protocol string) string {
	return "KUBE-SVC-" + portProtoHash(servicePortName, protocol)
}

// serviceFirewallChainName takes the servicePortName for a service and
// returns the associated iptables chain.  This is computed by hashing (sha256)
// then encoding to base32 and truncating with the prefix "KUBE-FW-".
func serviceFirewallChainName(servicePortName string, protocol string) string {
	return "KUBE-FW-" + portProtoHash(servicePortName, protocol)
}

// serviceLBPortChainName takes the servicePortName for a service and
// returns the associated iptables chain.  This is computed by hashing (sha256)
// then encoding to base32 and truncating with the prefix "KUBE-XLB-".  We do
// this because IPTables Chain Names must be <= 28 chars long, and the longer
// they are the harder they are to read.
func serviceLBChainName(servicePortName string, protocol string) string {
	return "KUBE-XLB-" + portProtoHash(servicePortName, protocol)
}

// This is the same as servicePortChainName but with the endpoint included.
func servicePortEndpointChainName(servicePortName string, protocol string, endpoint string) string {
	hash := sha256.Sum256([]byte(servicePortName + protocol + endpoint))
	encoded := base32.StdEncoding.EncodeToString(hash[:])
	return "KUBE-SEP-" + encoded[:16]
}

type endpointServicePair struct {
	endpoint        string
	servicePortName servicePortName
}

func (esp *endpointServicePair) IPPart() string {
	if index := strings.Index(esp.endpoint, ":"); index != -1 {
		return esp.endpoint[0:index]
	}
	return esp.endpoint
}

func shouldSkipService(svcName types.NamespacedName, service *v1.Service) bool {
	// if ClusterIP is "None" or empty, skip proxying
	if service.Spec.ClusterIP == v1.ClusterIPNone || service.Spec.ClusterIP == "" {
		glog.V(3).Infof("Skipping service %s due to clusterIP = %q", svcName, service.Spec.ClusterIP)
		return true
	}
	// Even if ClusterIP is set, ServiceTypeExternalName services don't get proxied
	if service.Spec.Type == v1.ServiceTypeExternalName {
		glog.V(3).Infof("Skipping service %s due to Type=ExternalName", svcName)
		return true
	}
	return false
}

// requestsOnlyLocalTraffic checks if service requests OnlyLocal traffic.
func requestsOnlyLocalTraffic(service *v1.Service) bool {
	if service.Spec.Type != v1.ServiceTypeLoadBalancer &&
		service.Spec.Type != v1.ServiceTypeNodePort {
		return false
	}

	// First check the beta annotation and then the first class field. This is so that
	// existing Services continue to work till the user decides to transition to the
	// first class field.
	if l, ok := service.Annotations[v1.BetaAnnotationExternalTraffic]; ok {
		switch l {
		case v1.AnnotationValueExternalTrafficLocal:
			return true
		case v1.AnnotationValueExternalTrafficGlobal:
			return false
		default:
			glog.Errorf("Invalid value for annotation %v: %v", v1.BetaAnnotationExternalTraffic, l)
			return false
		}
	}
	return service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal
}

// needsHealthCheck Check if service needs health check.
func needsHealthCheck(service *v1.Service) bool {
	if service.Spec.Type != v1.ServiceTypeLoadBalancer {
		return false
	}
	return requestsOnlyLocalTraffic(service)
}

// getServiceHealthCheckNodePort Return health check node port for service, if one exists
func getServiceHealthCheckNodePort(service *v1.Service) int32 {
	// First check the beta annotation and then the first class field. This is so that
	// existing Services continue to work till the user decides to transition to the
	// first class field.
	if l, ok := service.Annotations[v1.BetaAnnotationHealthCheckNodePort]; ok {
		p, err := strconv.Atoi(l)
		if err != nil {
			glog.Errorf("Failed to parse annotation %v: %v", v1.BetaAnnotationHealthCheckNodePort, err)
			return 0
		}
		return int32(p)
	}
	return service.Spec.HealthCheckNodePort
}

func loadBalancerStatusDeepCopy(lb *v1.LoadBalancerStatus) *v1.LoadBalancerStatus {
	c := &v1.LoadBalancerStatus{}
	c.Ingress = make([]v1.LoadBalancerIngress, len(lb.Ingress))
	for i := range lb.Ingress {
		c.Ingress[i] = lb.Ingress[i]
	}
	return c
}

func getRouterNetns(routerID string) string {
	return "qrouter-" + routerID
}

func probability(n int) string {
	return fmt.Sprintf("%0.5f", 1.0/float64(n))
}
