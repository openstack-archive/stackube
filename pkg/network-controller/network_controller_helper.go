package network

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	tprv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	drivertypes "git.openstack.org/openstack/stackube/pkg/openstack/types"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
)

const (
	networkPrefix = "network"
	subnetSuffix  = "subnet"
)

func (c *NetworkController) addNetworkToNeutron(kubeNetwork *tprv1.Network) {
	// The tenant name is the same with namespace, let's get tenantID by tenantName
	tenantName := kubeNetwork.GetNamespace()
	tenantID, err := c.Driver.GetTenantIDFromName(tenantName)
	if err != nil {
		// Retry for a while if failed
		err = wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			glog.V(3).Infof("failed to fetch tenantID for tenantName: %v, retrying\n", tenantName)
			if tenantID, err = c.Driver.GetTenantIDFromName(kubeNetwork.GetNamespace()); err != nil {
				return false, nil
			}
			return true, nil
		})
	}
	if err != nil {
		glog.Errorf("failed to fetch tenantID for tenantName: %v, abort! \n", tenantName)
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

	newNetworkStatus := tprv1.NetworkActive

	glog.V(4).Infof("[NetworkController]: adding network %s", driverNetwork.Name)

	// Check if tenant id exist
	check, err := c.Driver.CheckTenantID(driverNetwork.TenantID)
	if err != nil {
		glog.Errorf("[NetworkController]: check tenantID failed: %v", err)
	}
	if !check {
		glog.Warningf("[NetworkController]: tenantID %s doesn't exit in network provider", driverNetwork.TenantID)
		kubeNetwork.Status.State = tprv1.NetworkFailed
		c.updateNetwork(kubeNetwork)
		return
	}

	// Check if provider network id exist
	if kubeNetwork.Spec.NetworkID != "" {
		_, err := c.Driver.GetNetworkByID(kubeNetwork.Spec.NetworkID)
		if err != nil {
			glog.Warningf("[NetworkController]: network %s doesn't exit in network provider", kubeNetwork.Spec.NetworkID)
			newNetworkStatus = tprv1.NetworkFailed
		}
	} else {
		if len(driverNetwork.Subnets) == 0 {
			glog.Warningf("[NetworkController]: subnets of %s is null", driverNetwork.Name)
			newNetworkStatus = tprv1.NetworkFailed
		} else {
			// Check if provider network has already created
			_, err := c.Driver.GetNetwork(networkName)
			if err == nil {
				glog.Infof("[NetworkController]: network %s has already created", networkName)
			} else if err.Error() == util.ErrNotFound.Error() {
				// Create a new network by network provider
				err := c.Driver.CreateNetwork(driverNetwork)
				if err != nil {
					glog.Warningf("[NetworkController]: create network %s failed: %v", driverNetwork.Name, err)
					newNetworkStatus = tprv1.NetworkFailed
				}
			} else {
				glog.Warningf("[NetworkController]: get network failed: %v", err)
				newNetworkStatus = tprv1.NetworkFailed
			}
		}
	}

	kubeNetwork.Status.State = newNetworkStatus
	c.updateNetwork(kubeNetwork)
}

// updateNetwork updates Network TPR object by given object
func (c *NetworkController) updateNetwork(network *tprv1.Network) {
	err := c.NetworkClient.Put().
		Name(network.ObjectMeta.Name).
		Namespace(network.ObjectMeta.Namespace).
		Resource(tprv1.NetworkResourcePlural).
		Body(network).
		Do().
		Error()

	if err != nil {
		glog.Errorf("ERROR updating network status: %v\n", err)
	} else {
		glog.V(3).Infof("UPDATED network status: %#v\n", network)
	}
}
