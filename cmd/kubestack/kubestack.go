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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"git.openstack.org/openstack/stackube/pkg/kubestack/plugins"
	kubestacktypes "git.openstack.org/openstack/stackube/pkg/kubestack/types"
	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	cniSpecVersion "github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/golang/glog"

	// import plugins
	_ "git.openstack.org/openstack/stackube/pkg/kubestack/plugins/openvswitch"
)

var (
	// VERSION is filled out during the build process (using git describe output)
	VERSION = "1.0beta"

	netnsBasePath = "/var/run/netns"
)

// OpenStack describes openstack client and its plugins.
type OpenStack struct {
	Client openstack.Interface
	Plugin plugins.PluginInterface
}

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func loadNetConf(bytes []byte) (*kubestacktypes.NetConf, string, error) {
	n := &kubestacktypes.NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, "", fmt.Errorf("failed to load netconf: %v", err)
	}
	return n, n.CNIVersion, nil
}

func (os *OpenStack) getNetworkIDByNamespace(namespace string) (string, error) {
	// Only support one network and network's name is same with namespace.
	// TODO: make it general after multi-network is supported.
	networkName := util.BuildNetworkName(namespace, namespace)
	network, err := os.Client.GetNetworkByName(networkName)
	if err != nil {
		glog.Errorf("Get network by name %q failed: %v", networkName, err)
		return "", err
	}

	return network.Uid, nil
}

func getHostName() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return host
}

func getK8sArgs(args string) (string, string, error) {
	k8sArgs := kubestacktypes.K8sArgs{}
	if err := types.LoadArgs(args, &k8sArgs); err != nil {
		return "", "", err
	}
	return string(k8sArgs.K8S_POD_NAME), string(k8sArgs.K8S_POD_NAMESPACE), nil
}

func initOpenstack(stdinData []byte) (OpenStack, string, error) {
	// Load cni net config
	n, cniVersion, err := loadNetConf(stdinData)
	if err != nil {
		return OpenStack{}, "", err
	}

	//Init openstack client
	if n.KubestackConfig == "" {
		return OpenStack{}, "", fmt.Errorf("kubestack-config not specified")
	}

	if n.KubernetesConfig == "" {
		return OpenStack{}, "", fmt.Errorf("kubernetes-config not specified")
	}

	openStackClient, err := openstack.NewClient(n.KubestackConfig, n.KubernetesConfig)
	if err != nil {
		return OpenStack{}, "", err
	}

	os := OpenStack{
		Client: openStackClient,
	}

	// Init plugin
	pluginName := os.Client.GetPluginName()
	if pluginName == "" {
		pluginName = "ovs"
	}
	integrationBridge := os.Client.GetIntegrationBridge()
	if integrationBridge == "" {
		integrationBridge = "br-int"
	}
	plugin, _ := plugins.GetNetworkPlugin(pluginName)
	if plugin == nil {
		return OpenStack{}, "", fmt.Errorf("plugin %q not found", pluginName)
	}
	plugin.Init(integrationBridge)
	os.Plugin = plugin

	return os, cniVersion, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	osClient, cniVersion, err := initOpenstack(args.StdinData)
	if err != nil {
		glog.Errorf("Init OpenStack failed: %v", err)
		return err
	}

	// Get k8s args
	podName, podNamespace, err := getK8sArgs(args.Args)
	if err != nil {
		glog.Errorf("GetK8sArgs failed: %v", err)
		return err
	}

	// Get tenantID
	tenantID, err := osClient.Client.GetTenantIDFromName(podNamespace)
	if err != nil {
		glog.Errorf("Get tenantID failed: %v", err)
		return err
	}

	// Get networkID
	networkID, err := osClient.getNetworkIDByNamespace(podNamespace)
	if err != nil {
		glog.Errorf("Get networkID failed: %v", err)
		return err
	}

	// Build port name
	portName := util.BuildPortName(podNamespace, podName)
	podFullName := util.BuildFullPodName(podNamespace, podName)

	// Get port from openstack.
	port, err := osClient.Client.GetPort(portName)
	if err == util.ErrNotFound || port == nil {
		// Port not found, create a new one.
		portWithBinding, err := osClient.Client.CreatePort(networkID, tenantID, portName)
		if err != nil {
			glog.Errorf("CreatePort failed: %v", err)
			return err
		}
		port = &portWithBinding.Port
	} else if err != nil {
		glog.Errorf("GetPort failed: %v", err)
		return err
	}
	defer func() {
		if err != nil {
			if osClient.Client.DeletePortByID(port.ID) != nil {
				glog.Warningf("Delete port %s failed", port.ID)
			}
		}
	}()

	deviceOwner := fmt.Sprintf("compute:%s", getHostName())
	if port.DeviceOwner != deviceOwner {
		err := osClient.Client.UpdatePortsBinding(port.ID, deviceOwner)
		if err != nil {
			glog.Errorf("Update port %s failed: %v", portName, err)
			return err
		}
	}
	glog.V(4).Infof("Pod %s's port is %v", podName, port)

	// Get subnet and gateway
	subnet, err := osClient.Client.GetProviderSubnet(port.FixedIPs[0].SubnetID)
	if err != nil {
		glog.Errorf("Get info of subnet %s failed: %v", port.FixedIPs[0].SubnetID, err)
		return err
	}

	// Get network namespace.
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	// Setup interface for pod
	var netnsName string
	_, cidr, _ := net.ParseCIDR(subnet.Cidr)
	prefixSize, _ := cidr.Mask.Size()
	if strings.HasPrefix(netnsBasePath, netns.Path()) {
		// container runtime has already made the symlink for netns.
		netnsName = path.Base(netns.Path())
	} else {
		netnsName = podFullName
		destPath := filepath.Join(netnsBasePath, netnsName)
		if err := util.NetnsSymlink(netns.Path(), destPath); err != nil {
			return fmt.Errorf("error of symlink %q: %v", destPath, err)
		}

		defer func() {
			if err != nil {
				if _, err := os.Stat(destPath); !os.IsNotExist(err) {
					if err = os.Remove(destPath); err != nil && !os.IsNotExist(err) {
						glog.Warningf("Failed to remove netns symlink %q: %v", destPath, err)
					}
				}
			}
		}()
	}

	brInterface, conInterface, err := osClient.Plugin.SetupInterface(portName, args.ContainerID, port,
		fmt.Sprintf("%s/%d", port.FixedIPs[0].IPAddress, prefixSize),
		subnet.Gateway, args.IfName, netnsName)
	if err != nil {
		glog.Errorf("SetupInterface failed: %v", err)
		return err
	}

	// Collect the result in this variable - this is ultimately what gets "returned"
	// by this function by printing it to stdout.
	result := &current.Result{}
	// Populate container interface sandbox path
	conInterface.Sandbox = netns.Path()

	// Populate result.Interfaces
	result.Interfaces = []*current.Interface{brInterface, conInterface}
	// Populate result.IPs
	ip := net.ParseIP(port.FixedIPs[0].IPAddress)
	ipCidr := net.IPNet{
		IP:   ip,
		Mask: cidr.Mask,
	}
	gateway := net.ParseIP(subnet.Gateway)
	containerIPConfig := &current.IPConfig{
		Version: "4",
		Address: ipCidr,
		Gateway: gateway,
	}
	result.IPs = []*current.IPConfig{containerIPConfig}

	// Print result to stdout, in the format defined by the requested cniVersion.
	return types.PrintResult(result, cniVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	osClient, _, err := initOpenstack(args.StdinData)
	if err != nil {
		glog.Errorf("Init OpenStack failed: %v", err)
		return err
	}

	// Get k8s args
	podName, podNamespace, err := getK8sArgs(args.Args)
	if err != nil {
		glog.Errorf("GetK8sArgs failed: %v", err)
		return err
	}

	// Build port name
	portName := util.BuildPortName(podNamespace, podName)
	podFullName := util.BuildFullPodName(podNamespace, podName)

	// Get port from openstack
	port, err := osClient.Client.GetPort(portName)
	if err != nil {
		glog.Errorf("GetPort %s failed: %v", portName, err)
		return err
	}
	if port == nil {
		glog.Warningf("Port %s already deleted", portName)
		return nil
	}
	glog.V(4).Infof("Pod %s's port is %v", podName, port)

	// Delete interface
	err = osClient.Plugin.DestroyInterface(portName, args.ContainerID, port)
	if err != nil {
		glog.Errorf("DestroyInterface for pod %s failed: %v", podName, err)
		return err
	}

	// Delete port from openstack
	err = osClient.Client.DeletePortByName(portName)
	if err != nil {
		glog.Errorf("Delete port %s failed: %v", portName, err)
		return err
	}

	// Remove netns symlink.
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		glog.Warningf("failed to open netns %q: %v, suppose already deleted", args.Netns, err)
		return nil
	}
	defer netns.Close()
	if !strings.HasPrefix(netnsBasePath, netns.Path()) {
		destPath := filepath.Join(netnsBasePath, podFullName)
		if _, err := os.Stat(destPath); !os.IsNotExist(err) {
			if err = os.Remove(destPath); err != nil {
				glog.Warningf("failed to remove %q: %v", destPath, err)
			}
		}
	}

	return nil
}

// AddIgnoreUnknownArgs appends the 'IgnoreUnknown=1' option to CNI_ARGS before
// calling the IPAM plugin. Otherwise, it will complain about the Kubernetes
// arguments. See https://github.com/kubernetes/kubernetes/pull/24983
func AddIgnoreUnknownArgs() error {
	cniArgs := "IgnoreUnknown=1"
	if os.Getenv("CNI_ARGS") != "" {
		cniArgs = fmt.Sprintf("%s;%s", cniArgs, os.Getenv("CNI_ARGS"))
	}
	return os.Setenv("CNI_ARGS", cniArgs)
}

func main() {
	// Display the version on "--version", otherwise just delegate to the skel code.
	// Use a new flag set so as not to conflict with existing libraries which use "flag"
	flagSet := flag.NewFlagSet("kubestack", flag.ExitOnError)

	version := flagSet.Bool("version", false, "Display version")
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	if err := AddIgnoreUnknownArgs(); err != nil {
		os.Exit(1)
	}

	skel.PluginMain(cmdAdd, cmdDel, cniSpecVersion.All)
}
