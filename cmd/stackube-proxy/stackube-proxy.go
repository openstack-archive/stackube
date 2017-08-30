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
	"fmt"

	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/proxy"
	"git.openstack.org/openstack/stackube/pkg/util"
	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"os"
)

var (
	kubeconfig = pflag.String("kubeconfig", "/etc/kubernetes/admin.conf",
		"path to kubernetes admin config file")
	cloudconfig = pflag.String("cloudconfig", "/etc/stackube.conf",
		"path to stackube config file")
	version = pflag.Bool("version", false, "Display version")
	VERSION = "1.0beta"
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

	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

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
