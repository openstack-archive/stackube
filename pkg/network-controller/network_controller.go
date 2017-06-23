package network

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	tprv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	tprclient "git.openstack.org/openstack/stackube/pkg/network-controller/client"
	driver "git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
)

// Watcher is an network of watching on resource create/update/delete events
type NetworkController struct {
	NetworkClient *rest.RESTClient
	NetworkScheme *runtime.Scheme
	Driver        *driver.Client
}

func (c *NetworkController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	source := cache.NewListWatchFromClient(
		c.NetworkClient,
		tprv1.NetworkResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())

	_, networkInformer := cache.NewInformer(
		source,
		&tprv1.Network{},
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

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func NewNetworkController(kubeconfig, openstackConfigFile string) (*NetworkController, error) {
	// Create OpenStack client from config
	openstack, err := driver.NewClient(openstackConfigFile)
	if err != nil {
		return nil, fmt.Errorf("Couldn't initialize openstack: %#v", err)
	}

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubeclient from config: %v", err)
	}

	// initialize third party resource if it does not exist
	err = tprclient.CreateNetworkTPR(clientset)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create TPR to kube-apiserver: %v", err)
	}

	// make a new config for our extension's API group, using the first config as a baseline
	networkClient, networkScheme, err := tprclient.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for TPR: %v", err)
	}

	// wait until TPR gets processed
	err = tprclient.WaitForNetworkResource(networkClient)
	if err != nil {
		return nil, fmt.Errorf("failed to wait TPR change to ready status: %v", err)
	}

	networkController := &NetworkController{
		NetworkClient: networkClient,
		NetworkScheme: networkScheme,
		Driver:        openstack,
	}
	return networkController, nil
}

func (c *NetworkController) onAdd(obj interface{}) {
	network := obj.(*tprv1.Network)
	glog.Infof("[NETWORK CONTROLLER] OnAdd %s\n", network.ObjectMeta.SelfLink)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use networkScheme.Copy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	copyObj, err := c.NetworkScheme.Copy(network)
	if err != nil {
		glog.Errorf("ERROR creating a deep copy of network object: %v\n", err)
		return
	}

	networkCopy := copyObj.(*tprv1.Network)

	// This will:
	// 1. Create Network in Neutron
	// 2. Update Network TRP object status to Active or Failed
	c.addNetworkToNeutron(networkCopy)
}

func (c *NetworkController) onUpdate(oldObj, newObj interface{}) {
	// NOTE(harry) not supported yet
}

func (c *NetworkController) onDelete(obj interface{}) {
	if net, ok := obj.(*tprv1.Network); ok {
		glog.V(4).Infof("NetworkController: network %s deleted", net.Name)
		if net.Spec.NetworkID == "" {
			networkName := util.BuildNetworkName(net.GetNamespace(), net.GetName())
			err := c.Driver.DeleteNetwork(networkName)
			if err != nil {
				glog.Errorf("NetworkController: delete network %s failed in networkprovider: %v", networkName, err)
			} else {
				glog.V(4).Infof("NetworkController: network %s deleted in networkprovider", networkName)
			}
		}
	}
}
