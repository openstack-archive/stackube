package network

import (
	"time"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	drivertypes "git.openstack.org/openstack/stackube/pkg/openstack/types"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	networkPrefix = "network"
	subnetSuffix  = "subnet"
)

func (c *NetworkController) addNetworkToDriver(kubeNetwork *crv1.Network) {
	// The tenant name is the same with namespace, let's get tenantID by tenantName
	tenantName := kubeNetwork.GetNamespace()
	tenantID, err := c.driver.GetTenantIDFromName(tenantName)

	// Retry for a while if fetch tenantID failed or tenantID not found,
	// this is normally caused by cloud provider processing
	if err != nil || tenantID == "" {
		err = wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			tenantID, err = c.driver.GetTenantIDFromName(kubeNetwork.GetNamespace())
			if err != nil || tenantID == "" {
				glog.V(5).Infof("failed to fetch tenantID for tenantName: %v, retrying\n", tenantName)
				return false, nil
			}
			return true, nil
		})
	}
	if err != nil || tenantID == "" {
		glog.Errorf("failed to fetch tenantID for tenantName: %v, abort! \n", tenantName)
		return
	} else {
		glog.V(3).Infof("Got tenantID: %v for tenantName: %v", tenantID, tenantName)
	}

	networkName := util.BuildNetworkName(tenantName, kubeNetwork.GetName())

	// Translate Kubernetes network to OpenStack network
	driverNetwork := &drivertypes.Network{
		Name:     networkName,
		TenantID: tenantID,
		Subnets: []*drivertypes.Subnet{
			{
				// network: subnet = 1:1
				Name:     networkName + "-" + subnetSuffix,
				Cidr:     kubeNetwork.Spec.CIDR,
				Gateway:  kubeNetwork.Spec.Gateway,
				Tenantid: tenantID,
			},
		},
	}

	newNetworkStatus := crv1.NetworkActive

	glog.V(4).Infof("[NetworkController]: adding network %s", driverNetwork.Name)

	// Check if tenant id exist
	check, err := c.driver.CheckTenantID(driverNetwork.TenantID)
	if err != nil {
		glog.Errorf("[NetworkController]: check tenantID failed: %v", err)
	}
	if !check {
		glog.Warningf("[NetworkController]: tenantID %s doesn't exist in network provider", driverNetwork.TenantID)
		kubeNetwork.Status.State = crv1.NetworkFailed
		c.updateNetwork(kubeNetwork)
		return
	}

	// Check if provider network id exist
	if kubeNetwork.Spec.NetworkID != "" {
		_, err := c.driver.GetNetworkByID(kubeNetwork.Spec.NetworkID)
		if err != nil {
			glog.Warningf("[NetworkController]: network %s doesn't exit in network provider", kubeNetwork.Spec.NetworkID)
			newNetworkStatus = crv1.NetworkFailed
		}
	} else {
		if len(driverNetwork.Subnets) == 0 {
			glog.Warningf("[NetworkController]: subnets of %s is null", driverNetwork.Name)
			newNetworkStatus = crv1.NetworkFailed
		} else {
			// Check if provider network has already created
			_, err := c.driver.GetNetwork(networkName)
			if err == nil {
				glog.Infof("[NetworkController]: network %s has already created", networkName)
			} else if err.Error() == util.ErrNotFound.Error() {
				// Create a new network by network provider
				err := c.driver.CreateNetwork(driverNetwork)
				if err != nil {
					glog.Warningf("[NetworkController]: create network %s failed: %v", driverNetwork.Name, err)
					newNetworkStatus = crv1.NetworkFailed
				}
			} else {
				glog.Warningf("[NetworkController]: get network failed: %v", err)
				newNetworkStatus = crv1.NetworkFailed
			}
		}
	}

	kubeNetwork.Status.State = newNetworkStatus
	c.updateNetwork(kubeNetwork)
}

// updateNetwork updates Network CRD object by given object
func (c *NetworkController) updateNetwork(network *crv1.Network) {
	err := c.networkClient.Put().
		Name(network.Name).
		Namespace(network.Namespace).
		Resource(crv1.NetworkResourcePlural).
		Body(network).
		Do().
		Error()

	if err != nil {
		glog.Errorf("ERROR updating network status: %v\n", err)
	} else {
		glog.V(3).Infof("UPDATED network status: %#v\n", network)
	}
}
