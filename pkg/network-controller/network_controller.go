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

package network

import (
	"fmt"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/kubecrd"
	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"
)

// Watcher is an network of watching on resource create/update/delete events
type NetworkController struct {
	kubeCRDClient   *kubecrd.CRDClient
	driver          *openstack.Client
	networkInformer cache.Controller
}

func (c *NetworkController) GetKubeCRDClient() *kubecrd.CRDClient {
	return c.kubeCRDClient
}

func (c *NetworkController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	go c.networkInformer.Run(stopCh)
	<-stopCh

	return nil
}

func NewNetworkController(osClient *openstack.Client, kubeExtClient *apiextensionsclient.Clientset) (*NetworkController, error) {
	// initialize CRD if it does not exist
	_, err := kubecrd.CreateNetworkCRD(kubeExtClient)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create CRD to kube-apiserver: %v", err)
	}

	source := cache.NewListWatchFromClient(
		osClient.CRDClient.Client,
		crv1.NetworkResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())
	networkController := &NetworkController{
		kubeCRDClient: osClient.CRDClient,
		driver:        osClient,
	}
	_, networkInformer := cache.NewInformer(
		source,
		&crv1.Network{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    networkController.onAdd,
			UpdateFunc: networkController.onUpdate,
			DeleteFunc: networkController.onDelete,
		})
	networkController.networkInformer = networkInformer

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
