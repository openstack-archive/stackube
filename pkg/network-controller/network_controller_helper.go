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
	"bytes"
	"fmt"
	"html/template"
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

func (c *NetworkController) addNetworkToDriver(kubeNetwork *crv1.Network) error {
	// The tenant name is the same with namespace, let's get tenantID by tenantName
	tenantName := kubeNetwork.GetNamespace()
	tenantID, err := c.driver.GetTenantIDFromName(tenantName)

	// Retry for a while if fetch tenantID failed or tenantID not found,
	// this is normally caused by cloud provider processing
	if err != nil || tenantID == "" {
		err = wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			tenantID, err = c.driver.GetTenantIDFromName(kubeNetwork.GetNamespace())
			if err != nil {
				glog.Errorf("failed to fetch tenantID for tenantName: %v, error: %v retrying\n", tenantName, err)
				return false, err
			}

			if tenantID == "" {
				glog.V(5).Infof("tenantID is empty for tenantName: %v, retrying\n", tenantName)
				return false, err
			}
			return true, nil
		})
	}
	if err != nil || tenantID == "" {
		return fmt.Errorf("failed to fetch tenantID for tenantName: %v, error: %v abort! \n", tenantName, err)
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
		return err
	}
	if !check {
		kubeNetwork.Status.State = crv1.NetworkFailed
		c.kubeCRDClient.UpdateNetwork(kubeNetwork)
		return fmt.Errorf("tenantID %s doesn't exist in network provider", driverNetwork.TenantID)
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
	c.kubeCRDClient.UpdateNetwork(kubeNetwork)
	return nil
}

func parseTemplate(strtmpl string, obj interface{}) ([]byte, error) {
	var buf bytes.Buffer
	tmpl, err := template.New("template").Parse(strtmpl)
	if err != nil {
		return nil, fmt.Errorf("error when parsing template: %v", err)
	}
	err = tmpl.Execute(&buf, obj)
	if err != nil {
		return nil, fmt.Errorf("error when executing template: %v", err)
	}
	return buf.Bytes(), nil
}
