package main

import (
	"fmt"

	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/proxy"
	"git.openstack.org/openstack/stackube/pkg/util"
	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

var (
	kubeconfig = pflag.String("kubeconfig", "/etc/kubernetes/admin.conf",
		"path to kubernetes admin config file")
	cloudconfig = pflag.String("cloudconfig", "/etc/stackube.conf",
		"path to stackube config file")
)

func verifyClientSetting() error {
	config, err := util.NewClusterConfig(*kubeconfig)
	if err != nil {
		return fmt.Errorf("Init kubernetes cluster failed: %v", err)
	}

	_, err = kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("Init kubernetes clientset failed: %v", err)
	}

	_, err = openstack.NewClient(*cloudconfig, *kubeconfig)
	if err != nil {
		return fmt.Errorf("Init openstack client failed: %v", err)
	}

	return nil
}

func main() {
	util.InitFlags()
	util.InitLogs()
	defer util.FlushLogs()

	// Verify client setting at the beginning and fail early if there are errors.
	err := verifyClientSetting()
	if err != nil {
		glog.Fatal(err)
	}

	proxier, err := proxy.NewProxier(*kubeconfig, *cloudconfig)
	if err != nil {
		glog.Fatal(err)
	}

	proxier.RegisterInformers()

	go proxier.StartNamespaceInformer(wait.NeverStop)
	go proxier.StartServiceInformer(wait.NeverStop)
	go proxier.StartEndpointInformer(wait.NeverStop)
	go proxier.StartInformerFactory(wait.NeverStop)

	if err := proxier.SyncLoop(); err != nil {
		glog.Fatal(err)
	}
}
