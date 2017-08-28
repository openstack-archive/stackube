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
	"os"
	"time"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/kubecrd"
	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"
)

const (
	defaultDNSDomain    = "cluster.local"
	defaultKubeDNSImage = "stackube/k8s-dns-kube-dns-amd64:1.14.4"
	defaultDNSMasqImage = "stackube/k8s-dns-dnsmasq-nanny-amd64:1.14.4"
	defaultSideCarImage = "stackube/k8s-dns-sidecar-amd64:1.14.4"
)

// NetworkController manages the life cycle of Network.
type NetworkController struct {
	k8sclient       kubernetes.Interface
	kubeCRDClient   kubecrd.Interface
	driver          openstack.Interface
	networkInformer cache.Controller
}

// Run the network controller.
func (c *NetworkController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	go c.networkInformer.Run(stopCh)
	<-stopCh

	return nil
}

// NewNetworkController creates a new NetworkController.
func NewNetworkController(kubeClient kubernetes.Interface, osClient openstack.Interface, kubeExtClient *apiextensionsclient.Clientset) (*NetworkController, error) {
	// initialize CRD if it does not exist
	_, err := kubecrd.CreateNetworkCRD(kubeExtClient)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create CRD to kube-apiserver: %v", err)
	}

	source := cache.NewListWatchFromClient(
		osClient.GetCRDClient().Client(),
		crv1.NetworkResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())
	networkController := &NetworkController{
		k8sclient:     kubeClient,
		kubeCRDClient: osClient.GetCRDClient(),
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
	copyObj, err := c.kubeCRDClient.Scheme().Copy(network)
	if err != nil {
		glog.Errorf("ERROR creating a deep copy of network object: %v\n", err)
		return
	}

	networkCopy := copyObj.(*crv1.Network)

	// This will:
	// 1. Create Network in Neutron
	// 2. Update Network CRD object status to Active or Failed
	err = c.addNetworkToDriver(networkCopy)
	if err != nil {
		glog.Errorf("Add network to driver failed: %v", err)
		return
	}

	// create kube-dns in this namespace.
	namespace := networkCopy.Namespace
	if err := c.createKubeDNSDeployment(namespace); err != nil {
		glog.Errorf("Create kube-dns deployment failed: %v", err)
		return
	}

	if err := c.createKubeDNSService(namespace); err != nil {
		glog.Errorf("Create kube-dns service failed: %v", err)
		return
	}
}

func (c *NetworkController) onUpdate(oldObj, newObj interface{}) {
	// NOTE(harry) not supported yet
}

func (c *NetworkController) onDelete(obj interface{}) {
	net, ok := obj.(*crv1.Network)
	if !ok {
		glog.Warningf("Receiving an unkown object: %v", obj)
		return
	}

	glog.V(4).Infof("NetworkController: network %s deleted", net.Name)

	// Delete kube-dns deployment.
	if err := c.deleteDeployment(net.Namespace, "kube-dns"); err != nil {
		glog.Warningf("error on deleting kube-dns deployment: %v", err)
	}
	// Delete kube-dns services for non-system namespaces.
	if !util.IsSystemNamespace(net.Namespace) {
		if err := c.k8sclient.Core().Services(net.Namespace).Delete("kube-dns", apismetav1.NewDeleteOptions(0)); err != nil {
			glog.Warningf("error on deleting kube-dns service: %v", err)
		}
	}

	// Delete neutron network created by stackube.
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

func (c *NetworkController) createKubeDNSDeployment(namespace string) error {
	tempArgs := struct{ Namespace, DNSDomain, KubeDNSImage, DNSMasqImage, SidecarImage, KubernetesHost, KubernetesPort string }{
		Namespace:    namespace,
		DNSDomain:    defaultDNSDomain,
		KubeDNSImage: defaultKubeDNSImage,
		DNSMasqImage: defaultDNSMasqImage,
		SidecarImage: defaultSideCarImage,
	}
	if host := os.Getenv("KUBERNETES_SERVICE_HOST"); host != "" {
		tempArgs.KubernetesHost = host
	}
	if port := os.Getenv("KUBERNETES_SERVICE_PORT"); port != "" {
		tempArgs.KubernetesPort = port
	}
	dnsDeploymentBytes, err := parseTemplate(kubeDNSDeployment, tempArgs)
	kubeDNSDeploy := &v1beta1.Deployment{}
	if err = kuberuntime.DecodeInto(scheme.Codecs.UniversalDecoder(), dnsDeploymentBytes, kubeDNSDeploy); err != nil {
		return fmt.Errorf("unable to decode kube-dns deployment %v", err)
	}
	_, err = c.k8sclient.ExtensionsV1beta1().Deployments(namespace).Create(kubeDNSDeploy)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("unable to create a new kube-dns deployment: %v", err)
		}

		if _, err = c.k8sclient.ExtensionsV1beta1().Deployments(namespace).Update(kubeDNSDeploy); err != nil {
			return fmt.Errorf("unable to update the kube-dns deployment: %v", err)
		}
	}

	return nil
}

func (c *NetworkController) deleteDeployment(namespace, name string) error {
	if err := c.k8sclient.ExtensionsV1beta1().Deployments(namespace).Delete(name, apismetav1.NewDeleteOptions(0)); err != nil {
		return err
	}

	err := wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		_, err := c.k8sclient.ExtensionsV1beta1().Deployments(namespace).Get(name, apismetav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		}

		return false, nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *NetworkController) createKubeDNSService(namespace string) error {
	tempArgs := struct{ Namespace string }{
		Namespace: namespace,
	}
	dnsServiceBytes, err := parseTemplate(kubeDNSService, tempArgs)
	dnsService := &apiv1.Service{}
	if err = kuberuntime.DecodeInto(scheme.Codecs.UniversalDecoder(), dnsServiceBytes, dnsService); err != nil {
		return fmt.Errorf("unable to decode kube-dns service %v", err)
	}
	_, err = c.k8sclient.Core().Services(namespace).Create(dnsService)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("unable to create a new kube-dns service: %v", err)
		}

		if _, err = c.k8sclient.Core().Services(namespace).Update(dnsService); err != nil {
			return fmt.Errorf("unable to update the kube-dns service: %v", err)
		}
	}

	return nil
}
