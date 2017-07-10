package plugins

import (
	"fmt"
	"sync"

	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/golang/glog"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
)

type PluginInterface interface {
	SetupInterface(podName, podInfraContainerID string, port *ports.Port, ipcidr, gateway, ifName, netns string) (*current.Interface, *current.Interface, error)
	DestroyInterface(podName, podInfraContainerID string, port *ports.Port) error
	Init(integrationBridge string) error
}

// Factory is a function that returns a networkplugin.Interface.
// The config parameter provides an io.Reader handler to the factory in
// order to load specific configurations. If no configuration is provided
// the parameter is nil.
type Factory func() (PluginInterface, error)

// All registered network plugins.
var pluginsMutex sync.Mutex
var plugins = make(map[string]Factory)

// RegisterNetworkPlugin registers a networkplugin.Factory by name.  This
// is expected to happen during app startup.
func RegisterNetworkPlugin(name string, networkPlugin Factory) {
	pluginsMutex.Lock()
	defer pluginsMutex.Unlock()
	if _, found := plugins[name]; found {
		glog.Fatalf("Network plugin %q was registered twice", name)
	}
	glog.V(1).Infof("Registered network plugin %q", name)
	plugins[name] = networkPlugin
}

// GetNetworkPlugin creates an instance of the named network plugin, or nil if
// the name is not known.  The error return is only used if the named plugin
// was known but failed to initialize.
func GetNetworkPlugin(name string) (PluginInterface, error) {
	pluginsMutex.Lock()
	defer pluginsMutex.Unlock()
	f, found := plugins[name]
	if !found {
		return nil, nil
	}
	return f()
}

// InitNetworkPlugin creates an instance of the named networkPlugin plugin.
func InitNetworkPlugin(name string) (PluginInterface, error) {
	var networkPlugin PluginInterface

	if name == "" {
		glog.Info("No network plugin specified.")
		return nil, nil
	}

	var err error
	networkPlugin, err = GetNetworkPlugin(name)

	if err != nil {
		return nil, fmt.Errorf("could not init networkPlugin plugin %q: %v", name, err)
	}
	if networkPlugin == nil {
		return nil, fmt.Errorf("unknown networkPlugin plugin %q", name)
	}

	return networkPlugin, nil
}
