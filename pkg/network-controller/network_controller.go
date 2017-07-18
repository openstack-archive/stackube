package network

import (
	"fmt"

	apiv1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	crdClient "git.openstack.org/openstack/stackube/pkg/kubecrd"
	osDriver "git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
)

// Watcher is an network of watching on resource create/update/delete events
type NetworkController struct {
	kubeCRDClient *crdClient.CRDClient
	driver        *osDriver.Client
}

func (c *NetworkController) GetKubeCRDClient() *crdClient.CRDClient {
	return c.kubeCRDClient
}

func (c *NetworkController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	source := cache.NewListWatchFromClient(
		c.kubeCRDClient.Client,
		crv1.NetworkResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())

	_, networkInformer := cache.NewInformer(
		source,
		&crv1.Network{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		})

	go networkInformer.Run(stopCh)
	<-stopCh

	return nil
}

func NewNetworkController(kubeconfig, openstackConfigFile string) (*NetworkController, error) {
	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := util.NewClusterConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}
	clientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubeclient from config: %v", err)
	}

	// initialize CRD if it does not exist
	_, err = crdClient.CreateNetworkCRD(clientset)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create CRD to kube-apiserver: %v", err)
	}

	// Create OpenStack client from config
	openstack, err := osDriver.NewClient(openstackConfigFile, kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("Couldn't initialize openstack: %#v", err)
	}

	networkController := &NetworkController{
		kubeCRDClient: openstack.CRDClient,
		driver:        openstack,
	}
	return networkController, nil
}

func (c *NetworkController) onAdd(obj interface{}) {
	network := obj.(*crv1.Network)
	// glog.Infof("[NETWORK CONTROLLER] OnAdd %\n", network.ObjectMeta.SelfLink)
	glog.Infof("[NETWORK CONTROLLER] OnAdd %#v\n", network)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use networkScheme.Copy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	copyObj, err := c.GetKubeCRDClient().Scheme.Copy(network)
	if err != nil {
		glog.Errorf("ERROR creating a deep copy of network object: %v\n", err)
		return
	}

	networkCopy := copyObj.(*crv1.Network)

	// This will:
	// 1. Create Network in Neutron
	// 2. Update Network CRD object status to Active or Failed
	c.addNetworkToDriver(networkCopy)
}

func (c *NetworkController) onUpdate(oldObj, newObj interface{}) {
	// NOTE(harry) not supported yet
}

func (c *NetworkController) onDelete(obj interface{}) {
	if net, ok := obj.(*crv1.Network); ok {
		glog.V(4).Infof("NetworkController: network %s deleted", net.Name)
		if net.Spec.NetworkID == "" {
			networkName := util.BuildNetworkName(net.GetNamespace(), net.GetName())
			err := c.driver.DeleteNetwork(networkName)
			if err != nil {
				glog.Errorf("NetworkController: delete network %s failed in networkprovider: %v", networkName, err)
			} else {
				glog.V(4).Infof("NetworkController: network %s deleted in networkprovider", networkName)
			}
		}
	}
}
