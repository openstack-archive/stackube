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

package openvswitch

import (
	"fmt"
	"strings"

	"git.openstack.org/openstack/stackube/pkg/kubestack/plugins"
	"git.openstack.org/openstack/stackube/pkg/util"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/golang/glog"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
)

const (
	pluginName = "ovs"
)

type OVSPlugin struct {
	IntegrationBridge string
}

func init() {
	plugins.RegisterNetworkPlugin(pluginName, func() (plugins.PluginInterface, error) {
		return NewOVSPlugin(), nil
	})
}

func NewOVSPlugin() *OVSPlugin {
	return &OVSPlugin{}
}

func (p *OVSPlugin) Name() string {
	return pluginName
}

func (p *OVSPlugin) Init(integrationBridge string) error {
	p.IntegrationBridge = integrationBridge
	return nil
}

func (p *OVSPlugin) buildBridgeName(portID string) string {
	return ("qbr" + portID)[:14]
}

func (p *OVSPlugin) buildTapName(portID string) string {
	return ("tap" + portID)[:14]
}

func (p *OVSPlugin) buildSandboxInterfaceName(portID string) (string, string) {
	return ("vib" + portID)[:14], ("vif" + portID)[:14]
}

func (p *OVSPlugin) buildVethName(portID string) (string, string) {
	return ("qvb" + portID)[:14], ("qvo" + portID)[:14]
}

func (p *OVSPlugin) SetupSandboxInterface(podName, podInfraContainerID string, port *ports.Port, ipcidr, gateway, ifName, netns string) (*current.Interface, error) {
	vibName, vifName := p.buildSandboxInterfaceName(port.ID)
	ret, err := util.RunCommand("ip", "link", "add", vibName, "type", "veth", "peer", "name", vifName)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	bridge := p.buildBridgeName(port.ID)
	ret, err = util.RunCommand("brctl", "addif", bridge, vibName)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "link", "set", "dev", vifName, "address", port.MACAddress)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "link", "set", vifName, "netns", netns)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "netns", "exec", netns, "ip", "link", "set", vifName, "down")
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "netns", "exec", netns, "ip", "link", "set", vifName, "name", ifName)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "netns", "exec", netns, "ip", "link", "set", ifName, "up")
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "netns", "exec", netns, "ip", "addr", "add", "dev", ifName, ipcidr)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "netns", "exec", netns, "ip", "route", "add", "default", "via", gateway)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "link", "set", "dev", vibName, "up")
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	_, err = util.RunCommand("rm", "-f", fmt.Sprintf("/var/run/netns/%s", netns))
	if err != nil {
		glog.V(5).Infof("Warning: remove netns symlink failed: %v", err)
	}

	return &current.Interface{
		Name: p.buildTapName(port.ID),
		Mac:  port.MACAddress,
	}, nil
}

func (p *OVSPlugin) SetupOVSInterface(podName, podInfraContainerID string, port *ports.Port) (*current.Interface, error) {
	qvb, qvo := p.buildVethName(port.ID)
	ret, err := util.RunCommand("ip", "link", "add", qvb, "type", "veth", "peer", "name", qvo)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	bridge := p.buildBridgeName(port.ID)
	ret, err = util.RunCommand("brctl", "addbr", bridge)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "link", "set", qvb, "up")
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "link", "set", qvo, "up")
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ip", "link", "set", bridge, "up")
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("brctl", "addif", bridge, qvb)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	ret, err = util.RunCommand("ovs-vsctl", "-vconsole:off", "--", "--if-exists", "del-port",
		qvo, "--", "add-port", p.IntegrationBridge, qvo, "--", "set", "Interface", qvo,
		fmt.Sprintf("external_ids:attached-mac=%s", port.MACAddress),
		fmt.Sprintf("external_ids:iface-id=%s", port.ID),
		fmt.Sprintf("external_ids:vm-id=%s", podName),
		"external_ids:iface-status=active")
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}

	// Get bridge mac
	ret, err = util.RunCommand("ip", "link", "show", bridge)
	if err != nil {
		glog.Warningf("SetupInterface failed, ret:%s, error:%v", strings.Join(ret, "\n"), err)
		p.DestroyInterface(podName, podInfraContainerID, port)
		return nil, err
	}
	mac := ret[1][15:32]
	return &current.Interface{
		Name: bridge,
		Mac:  mac,
	}, nil
}

func (p *OVSPlugin) SetupInterface(podName, podInfraContainerID string, port *ports.Port, ipcidr, gateway, ifName, netns string) (*current.Interface, *current.Interface, error) {
	brInterface, err := p.SetupOVSInterface(podName, podInfraContainerID, port)
	if err != nil {
		glog.Errorf("SetupOVSInterface failed: %v", err)
		return nil, nil, err
	}

	conInterface, err := p.SetupSandboxInterface(podName, podInfraContainerID, port, ipcidr, gateway, ifName, netns)
	if err != nil {
		glog.Errorf("SetupSandboxInterface failed: %v", err)
		return nil, nil, err
	}

	glog.V(4).Infof("SetupInterface for %s done", podName)
	return brInterface, conInterface, nil
}

func (p *OVSPlugin) destroyOVSInterface(podName, portID string) error {
	_, qvo := p.buildVethName(portID)
	bridge := p.buildBridgeName(portID)

	output, err := util.RunCommand("ovs-vsctl", "-vconsole:off", "--if-exists", "del-port", qvo)
	if err != nil {
		glog.Warningf("Warning: ovs del-port %s failed: %v, %v", qvo, output, err)
	}

	output, err = util.RunCommand("ip", "link", "set", "dev", qvo, "down")
	if err != nil {
		glog.Warningf("Warning: set dev %s down failed: %v, %v", qvo, output, err)
	}

	output, err = util.RunCommand("ip", "link", "delete", "dev", qvo)
	if err != nil {
		glog.Warningf("Warning: delete dev %s failed: %v, %v", qvo, output, err)
	}

	output, err = util.RunCommand("ip", "link", "set", "dev", bridge, "down")
	if err != nil {
		glog.Warningf("Warning: set bridge %s down failed: %v, %v", bridge, output, err)
	}

	output, err = util.RunCommand("brctl", "delbr", bridge)
	if err != nil {
		glog.Warningf("Warning: delete bridge %s failed: %v, %v", bridge, output, err)
	}

	return nil
}

func (p *OVSPlugin) destroySandboxInterface(podName, podInfraContainerID, portID string) error {
	vibName, _ := p.buildSandboxInterfaceName(portID)
	_, err := util.RunCommand("ip", "link", "delete", vibName)
	if err != nil {
		glog.V(5).Infof("Warning: DestroyInterface failed: %v", err)
	}

	return nil
}

func (p *OVSPlugin) DestroyInterface(podName, podInfraContainerID string, port *ports.Port) error {
	p.destroyOVSInterface(podName, port.ID)
	p.destroySandboxInterface(podName, podInfraContainerID, port.ID)
	glog.V(4).Infof("DestroyInterface for %s done", podName)
	return nil
}
