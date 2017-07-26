package openstack

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/golang/glog"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/lbaas_v2/listeners"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/lbaas_v2/loadbalancers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/lbaas_v2/monitors"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/lbaas_v2/pools"
	"github.com/gophercloud/gophercloud/pagination"
)

const (
	defaultMonitorDelay   = 10
	defaultMonitorRetry   = 3
	defaultMonotorTimeout = 3

	// loadbalancerActive* is configuration of exponential backoff for
	// going into ACTIVE loadbalancer provisioning status. Starting with 1
	// seconds, multiplying by 1.2 with each step and taking 19 steps at maximum
	// it will time out after 128s, which roughly corresponds to 120s
	loadbalancerActiveInitDealy = 1 * time.Second
	loadbalancerActiveFactor    = 1.2
	loadbalancerActiveSteps     = 19

	// loadbalancerDelete* is configuration of exponential backoff for
	// waiting for delete operation to complete. Starting with 1
	// seconds, multiplying by 1.2 with each step and taking 13 steps at maximum
	// it will time out after 32s, which roughly corresponds to 30s
	loadbalancerDeleteInitDealy = 1 * time.Second
	loadbalancerDeleteFactor    = 1.2
	loadbalancerDeleteSteps     = 13

	activeStatus = "ACTIVE"
	errorStatus  = "ERROR"
)

// LoadBalancer contains all essential information of kubernetes service.
type LoadBalancer struct {
	Name            string
	ServicePort     int
	TenantID        string
	SubnetID        string
	Protocol        string
	InternalIP      string
	ExternalIP      string
	SessionAffinity bool
	Endpoints       []Endpoint
}

// Endpoint represents a container endpoint.
type Endpoint struct {
	Address string
	Port    int
}

// LoadBalancerStatus contains the status of a load balancer.
type LoadBalancerStatus struct {
	InternalIP string
	ExternalIP string
}

// EnsureLoadBalancer ensures a load balancer is created.
func (os *Client) EnsureLoadBalancer(lb *LoadBalancer) (*LoadBalancerStatus, error) {
	// removes old one if already exists.
	loadbalancer, err := os.getLoadBalanceByName(lb.Name)
	if err != nil {
		if isNotFound(err) {
			// create a new one.
			lbOpts := loadbalancers.CreateOpts{
				Name:        lb.Name,
				Description: "Stackube service",
				VipSubnetID: lb.SubnetID,
				TenantID:    lb.TenantID,
			}
			loadbalancer, err = loadbalancers.Create(os.Network, lbOpts).Extract()
			if err != nil {
				glog.Errorf("Create load balancer %q failed: %v", lb.Name, err)
				return nil, err
			}
		}
	} else {
		glog.V(3).Infof("LoadBalancer %s already exists", lb.Name)
	}

	status, err := os.waitLoadBalancerStatus(loadbalancer.ID)
	if err != nil {
		glog.Errorf("Waiting for load balancer provision failed: %v", err)
		return nil, err
	}

	glog.V(3).Infof("Load balancer %q becomes %q", lb.Name, status)

	// get old listeners
	var listener *listeners.Listener
	oldListeners, err := os.getListenersByLoadBalancerID(loadbalancer.ID)
	if err != nil {
		return nil, fmt.Errorf("error getting LB %s listeners: %v", loadbalancer.Name, err)
	}
	for i := range oldListeners {
		l := oldListeners[i]
		if l.ProtocolPort == lb.ServicePort {
			listener = &l
		} else {
			// delete the obsolete listener
			if err := os.ensureListenerDeleted(loadbalancer.ID, l); err != nil {
				return nil, fmt.Errorf("error deleting listener %q: %v", l.Name, err)
			}
			os.waitLoadBalancerStatus(loadbalancer.ID)
		}
	}

	// create the listener.
	if listener == nil {
		lisOpts := listeners.CreateOpts{
			LoadbalancerID: loadbalancer.ID,
			// Only tcp is supported now.
			Protocol:     listeners.ProtocolTCP,
			ProtocolPort: lb.ServicePort,
			TenantID:     lb.TenantID,
			Name:         lb.Name,
		}
		listener, err = listeners.Create(os.Network, lisOpts).Extract()
		if err != nil {
			glog.Errorf("Create listener %q failed: %v", lb.Name, err)
			return nil, err
		}
		os.waitLoadBalancerStatus(loadbalancer.ID)
	}

	// create the load balancer pool.
	pool, err := os.getPoolByListenerID(loadbalancer.ID, listener.ID)
	if err != nil && !isNotFound(err) {
		return nil, fmt.Errorf("error getting pool for listener %q: %v", listener.ID, err)
	}
	if pool == nil {
		poolOpts := pools.CreateOpts{
			Name:       lb.Name,
			ListenerID: listener.ID,
			Protocol:   pools.ProtocolTCP,
			LBMethod:   pools.LBMethodRoundRobin,
			TenantID:   lb.TenantID,
		}
		if lb.SessionAffinity {
			poolOpts.Persistence = &pools.SessionPersistence{Type: "SOURCE_IP"}
		}
		pool, err = pools.Create(os.Network, poolOpts).Extract()
		if err != nil {
			glog.Errorf("Create pool %q failed: %v", lb.Name, err)
			return nil, err
		}
		os.waitLoadBalancerStatus(loadbalancer.ID)
	}

	// create load balancer members.
	members, err := os.getMembersByPoolID(pool.ID)
	if err != nil && !isNotFound(err) {
		return nil, fmt.Errorf("error getting members for pool %q: %v", pool.ID, err)
	}
	for _, ep := range lb.Endpoints {
		if !memberExists(members, ep.Address, ep.Port) {
			memberName := fmt.Sprintf("%s-%s-%d", lb.Name, ep.Address, ep.Port)
			_, err = pools.CreateMember(os.Network, pool.ID, pools.CreateMemberOpts{
				Name:         memberName,
				ProtocolPort: ep.Port,
				Address:      ep.Address,
				SubnetID:     lb.SubnetID,
			}).Extract()
			if err != nil {
				glog.Errorf("Create member %q failed: %v", memberName, err)
				return nil, err
			}
			os.waitLoadBalancerStatus(loadbalancer.ID)
		} else {
			members = popMember(members, ep.Address, ep.Port)
		}
	}
	// delete obsolete members
	for _, member := range members {
		glog.V(4).Infof("Deleting obsolete member %s for pool %s address %s", member.ID,
			pool.ID, member.Address)
		err := pools.DeleteMember(os.Network, pool.ID, member.ID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return nil, fmt.Errorf("error deleting member %s for pool %s address %s: %v",
				member.ID, pool.ID, member.Address, err)
		}
	}

	// create loadbalancer monitor.
	if pool.MonitorID == "" {
		_, err = monitors.Create(os.Network, monitors.CreateOpts{
			Name:       lb.Name,
			Type:       monitors.TypeTCP,
			PoolID:     pool.ID,
			TenantID:   lb.TenantID,
			Delay:      defaultMonitorDelay,
			Timeout:    defaultMonotorTimeout,
			MaxRetries: defaultMonitorRetry,
		}).Extract()
		if err != nil {
			glog.Errorf("Create monitor for pool %q failed: %v", pool.ID, err)
			return nil, err
		}
	}

	// associate external IP for the vip.
	fip, err := os.associateFloatingIP(lb.TenantID, loadbalancer.VipPortID, lb.ExternalIP)
	if err != nil {
		glog.Errorf("associateFloatingIP for port %q failed: %v", loadbalancer.VipPortID, err)
		return nil, err
	}

	return &LoadBalancerStatus{
		InternalIP: loadbalancer.VipAddress,
		ExternalIP: fip,
	}, nil
}

// GetLoadBalancer gets a load balancer by name.
func (os *Client) GetLoadBalancer(name string) (*LoadBalancer, error) {
	// get load balancer
	lb, err := os.getLoadBalanceByName(name)
	if err != nil {
		return nil, err
	}

	// get listener
	listener, err := os.getListenerByName(name)
	if err != nil {
		return nil, err
	}

	// get members
	endpoints := make([]Endpoint, 0)
	for _, pool := range listener.Pools {
		for _, m := range pool.Members {
			endpoints = append(endpoints, Endpoint{
				Address: m.Address,
				Port:    m.ProtocolPort,
			})
		}
	}

	return &LoadBalancer{
		Name:            lb.Name,
		ServicePort:     listener.ProtocolPort,
		TenantID:        lb.TenantID,
		SubnetID:        lb.VipSubnetID,
		Protocol:        listener.Protocol,
		InternalIP:      lb.VipAddress,
		SessionAffinity: listener.Pools[0].Persistence.Type != "",
		Endpoints:       endpoints,
	}, nil
}

// LoadBalancerExist returns whether a load balancer has already been exist.
func (os *Client) LoadBalancerExist(name string) (bool, error) {
	_, err := os.getLoadBalanceByName(name)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// EnsureLoadBalancerDeleted ensures a load balancer is deleted.
func (os *Client) EnsureLoadBalancerDeleted(name string) error {
	// get load balancer
	lb, err := os.getLoadBalanceByName(name)
	if err != nil {
		if isNotFound(err) {
			return nil
		}

		return err
	}

	// delete floatingip
	floatingIP, err := os.getFloatingIPByPortID(lb.VipPortID)
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("error getting floating ip by port %q: %v", lb.VipPortID, err)
	}
	if floatingIP != nil {
		err = floatingips.Delete(os.Network, floatingIP.ID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("error deleting floating ip %q: %v", floatingIP.ID, err)
		}
	}

	// get listeners and corelative pools and members
	var poolIDs []string
	var monitorIDs []string
	var memberIDs []string
	listenerList, err := os.getListenersByLoadBalancerID(lb.ID)
	if err != nil {
		return fmt.Errorf("Error getting load balancer %s listeners: %v", lb.ID, err)
	}

	for _, listener := range listenerList {
		pool, err := os.getPoolByListenerID(lb.ID, listener.ID)
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("error getting pool for listener %s: %v", listener.ID, err)
		}
		poolIDs = append(poolIDs, pool.ID)
		if pool.MonitorID != "" {
			monitorIDs = append(monitorIDs, pool.MonitorID)
		}
	}
	for _, pool := range poolIDs {
		membersList, err := os.getMembersByPoolID(pool)
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("Error getting pool members %s: %v", pool, err)
		}
		for _, member := range membersList {
			memberIDs = append(memberIDs, member.ID)
		}
	}

	// delete all monitors
	for _, monitorID := range monitorIDs {
		err := monitors.Delete(os.Network, monitorID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return err
		}
		os.waitLoadBalancerStatus(lb.ID)
	}

	// delete all members and pools
	for _, poolID := range poolIDs {
		// delete all members for this pool
		for _, memberID := range memberIDs {
			err := pools.DeleteMember(os.Network, poolID, memberID).ExtractErr()
			if err != nil && !isNotFound(err) {
				return err
			}
			os.waitLoadBalancerStatus(lb.ID)
		}

		// delete pool
		err := pools.Delete(os.Network, poolID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return err
		}
		os.waitLoadBalancerStatus(lb.ID)
	}

	// delete all listeners
	for _, listener := range listenerList {
		err := listeners.Delete(os.Network, listener.ID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return err
		}
		os.waitLoadBalancerStatus(lb.ID)
	}

	// delete the load balancer
	err = loadbalancers.Delete(os.Network, lb.ID).ExtractErr()
	if err != nil && !isNotFound(err) {
		return err
	}
	os.waitLoadBalancerStatus(lb.ID)

	return nil
}

func (os *Client) ensureListenerDeleted(loadbalancerID string, listener listeners.Listener) error {
	for _, pool := range listener.Pools {
		for _, member := range pool.Members {
			// delete member
			if err := pools.DeleteMember(os.Network, pool.ID, member.ID).ExtractErr(); err != nil && !isNotFound(err) {
				return err
			}
			os.waitLoadBalancerStatus(loadbalancerID)
		}

		// delete monitor
		if err := monitors.Delete(os.Network, pool.MonitorID).ExtractErr(); err != nil && !isNotFound(err) {
			return err
		}
		os.waitLoadBalancerStatus(loadbalancerID)

		// delete pool
		if err := pools.Delete(os.Network, pool.ID).ExtractErr(); err != nil && !isNotFound(err) {
			return err
		}
		os.waitLoadBalancerStatus(loadbalancerID)
	}

	// delete listener
	if err := listeners.Delete(os.Network, listener.ID).ExtractErr(); err != nil && !isNotFound(err) {
		return err
	}
	os.waitLoadBalancerStatus(loadbalancerID)

	return nil
}

func (os *Client) waitLoadBalancerStatus(loadbalancerID string) (string, error) {
	backoff := wait.Backoff{
		Duration: loadbalancerActiveInitDealy,
		Factor:   loadbalancerActiveFactor,
		Steps:    loadbalancerActiveSteps,
	}

	var provisioningStatus string
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		loadbalancer, err := loadbalancers.Get(os.Network, loadbalancerID).Extract()
		if err != nil {
			return false, err
		}
		provisioningStatus = loadbalancer.ProvisioningStatus
		if loadbalancer.ProvisioningStatus == activeStatus {
			return true, nil
		} else if loadbalancer.ProvisioningStatus == errorStatus {
			return true, fmt.Errorf("Loadbalancer has gone into ERROR state")
		} else {
			return false, nil
		}

	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("Loadbalancer failed to go into ACTIVE provisioning status within alloted time")
	}
	return provisioningStatus, err
}

func (os *Client) waitLoadbalancerDeleted(loadbalancerID string) error {
	backoff := wait.Backoff{
		Duration: loadbalancerDeleteInitDealy,
		Factor:   loadbalancerDeleteFactor,
		Steps:    loadbalancerDeleteSteps,
	}
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		_, err := loadbalancers.Get(os.Network, loadbalancerID).Extract()
		if err != nil {
			if err == ErrNotFound {
				return true, nil
			} else {
				return false, err
			}
		} else {
			return false, nil
		}
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("Loadbalancer failed to delete within the alloted time")
	}

	return err
}

func (os *Client) getListenersByLoadBalancerID(id string) ([]listeners.Listener, error) {
	var existingListeners []listeners.Listener
	err := listeners.List(os.Network, listeners.ListOpts{LoadbalancerID: id}).EachPage(func(page pagination.Page) (bool, error) {
		listenerList, err := listeners.ExtractListeners(page)
		if err != nil {
			return false, err
		}
		for _, l := range listenerList {
			for _, lb := range l.Loadbalancers {
				if lb.ID == id {
					existingListeners = append(existingListeners, l)
					break
				}
			}
		}

		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return existingListeners, nil
}

func (os *Client) getLoadBalanceByName(name string) (*loadbalancers.LoadBalancer, error) {
	var lb *loadbalancers.LoadBalancer

	opts := loadbalancers.ListOpts{Name: name}
	pager := loadbalancers.List(os.Network, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		lbs, err := loadbalancers.ExtractLoadBalancers(page)
		if err != nil {
			return false, err
		}

		switch len(lbs) {
		case 0:
			return false, ErrNotFound
		case 1:
			lb = &lbs[0]
			return true, nil
		default:
			return false, ErrMultipleResults
		}
	})
	if err != nil {
		return nil, err
	}

	if lb == nil {
		return nil, ErrNotFound
	}

	return lb, nil
}

func (os *Client) getPoolByListenerID(loadbalancerID string, listenerID string) (*pools.Pool, error) {
	listenerPools := make([]pools.Pool, 0, 1)
	err := pools.List(os.Network, pools.ListOpts{LoadbalancerID: loadbalancerID}).EachPage(
		func(page pagination.Page) (bool, error) {
			poolsList, err := pools.ExtractPools(page)
			if err != nil {
				return false, err
			}
			for _, p := range poolsList {
				for _, l := range p.Listeners {
					if l.ID == listenerID {
						listenerPools = append(listenerPools, p)
					}
				}
			}
			if len(listenerPools) > 1 {
				return false, ErrMultipleResults
			}
			return true, nil
		})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(listenerPools) == 0 {
		return nil, ErrNotFound
	} else if len(listenerPools) > 1 {
		return nil, ErrMultipleResults
	}

	return &listenerPools[0], nil
}

// getPoolByName gets openstack pool by name.
func (os *Client) getPoolByName(name string) (*pools.Pool, error) {
	var pool *pools.Pool

	opts := pools.ListOpts{Name: name}
	pager := pools.List(os.Network, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		ps, err := pools.ExtractPools(page)
		if err != nil {
			return false, err
		}

		switch len(ps) {
		case 0:
			return false, ErrNotFound
		case 1:
			pool = &ps[0]
			return true, nil
		default:
			return false, ErrMultipleResults
		}
	})
	if err != nil {
		return nil, err
	}

	if pool == nil {
		return nil, ErrNotFound
	}

	return pool, nil
}

func (os *Client) getListenerByName(name string) (*listeners.Listener, error) {
	var listener *listeners.Listener

	opts := listeners.ListOpts{Name: name}
	pager := listeners.List(os.Network, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		lists, err := listeners.ExtractListeners(page)
		if err != nil {
			return false, err
		}

		switch len(lists) {
		case 0:
			return false, ErrNotFound
		case 1:
			listener = &lists[0]
			return true, nil
		default:
			return false, ErrMultipleResults
		}
	})
	if err != nil {
		return nil, err
	}

	if listener == nil {
		return nil, ErrNotFound
	}

	return listener, nil
}

func (os *Client) getMembersByPoolID(id string) ([]pools.Member, error) {
	var members []pools.Member
	err := pools.ListMembers(os.Network, id, pools.ListMembersOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		membersList, err := pools.ExtractMembers(page)
		if err != nil {
			return false, err
		}
		members = append(members, membersList...)

		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return members, nil
}

func (os *Client) getFloatingIPByPortID(portID string) (*floatingips.FloatingIP, error) {
	opts := floatingips.ListOpts{
		PortID: portID,
	}
	pager := floatingips.List(os.Network, opts)

	floatingIPList := make([]floatingips.FloatingIP, 0, 1)

	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		f, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, err
		}
		floatingIPList = append(floatingIPList, f...)
		if len(floatingIPList) > 1 {
			return false, ErrMultipleResults
		}
		return true, nil
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(floatingIPList) == 0 {
		return nil, ErrNotFound
	} else if len(floatingIPList) > 1 {
		return nil, ErrMultipleResults
	}

	return &floatingIPList[0], nil
}

// Check if a member exists for node
func memberExists(members []pools.Member, addr string, port int) bool {
	for _, member := range members {
		if member.Address == addr && member.ProtocolPort == port {
			return true
		}
	}

	return false
}

func (os *Client) associateFloatingIP(tenantID, portID, floatingIPAddress string) (string, error) {
	var fip *floatingips.FloatingIP
	opts := floatingips.ListOpts{FloatingIP: floatingIPAddress}
	pager := floatingips.List(os.Network, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		floatingipList, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, err
		}

		if len(floatingipList) > 0 {
			fip = &floatingipList[0]
		}

		return true, nil
	})
	if err != nil {
		return "", err
	}

	if fip != nil {
		if fip.PortID != "" {
			if fip.PortID == portID {
				glog.V(3).Infof("FIP %q has already been associated with port %q", floatingIPAddress, portID)
				return fip.FloatingIP, nil
			}
			// fip has already been used
			return fip.FloatingIP, fmt.Errorf("FloatingIP %v is already been binded to %v", floatingIPAddress, fip.PortID)
		}

		// Update floatingip
		floatOpts := floatingips.UpdateOpts{PortID: &portID}
		_, err = floatingips.Update(os.Network, fip.ID, floatOpts).Extract()
		if err != nil {
			glog.Errorf("Bind floatingip %v to %v failed: %v", floatingIPAddress, portID, err)
			return "", err
		}
	} else {
		// Create floatingip
		opts := floatingips.CreateOpts{
			FloatingNetworkID: os.ExtNetID,
			TenantID:          tenantID,
			FloatingIP:        floatingIPAddress,
			PortID:            portID,
		}
		fip, err = floatingips.Create(os.Network, opts).Extract()
		if err != nil {
			glog.Errorf("Create floatingip failed: %v", err)
			return "", err
		}
	}

	return fip.FloatingIP, nil
}

func popMember(members []pools.Member, addr string, port int) []pools.Member {
	for i, member := range members {
		if member.Address == addr && member.ProtocolPort == port {
			members[i] = members[len(members)-1]
			members = members[:len(members)-1]
		}
	}

	return members
}

func isNotFound(err error) bool {
	if err == ErrNotFound {
		return true
	}

	if _, ok := err.(*gophercloud.ErrResourceNotFound); ok {
		return true
	}

	if _, ok := err.(*gophercloud.ErrDefault404); ok {
		return true
	}

	return false
}
