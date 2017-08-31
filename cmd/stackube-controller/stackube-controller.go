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
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/tenant"
	"git.openstack.org/openstack/stackube/pkg/network-controller"
	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/service-controller"
	"git.openstack.org/openstack/stackube/pkg/util"

	extclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
)

var (
	kubeconfig = pflag.String("kubeconfig", "/etc/kubernetes/admin.conf",
		"path to kubernetes admin config file")
	cloudconfig = pflag.String("cloudconfig", "/etc/stackube.conf",
		"path to stackube config file")
	userCIDR    = pflag.String("user-cidr", "10.244.0.0/16", "user Pod network CIDR")
	userGateway = pflag.String("user-gateway", "10.244.0.1", "user Pod network gateway")
	version     = pflag.Bool("version", false, "Display version")
	VERSION     = "1.0beta"
)

func startControllers(kubeClient *kubernetes.Clientset,
	osClient openstack.Interface, kubeExtClient *extclientset.Clientset) error {
	// Creates a new Tenant controller
	tenantController, err := tenant.NewTenantController(kubeClient, osClient, kubeExtClient)
	if err != nil {
		return err
	}

	// Creates a new Network controller
	networkController, err := network.NewNetworkController(kubeClient, osClient, kubeExtClient)
	if err != nil {
		return err
	}

	// Creates a new RBAC controller
	rbacController, err := rbacmanager.NewRBACController(kubeClient, osClient.GetCRDClient(), *userCIDR, *userGateway)
	if err != nil {
		return err
	}

	// Creates a new service controller
	serviceController, err := service.NewServiceController(kubeClient, osClient)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg, ctx := errgroup.WithContext(ctx)

	// start auth controllers in stackube
	wg.Go(func() error { return tenantController.Run(ctx.Done()) })
	wg.Go(func() error { return rbacController.Run(ctx.Done()) })

	// start network controller
	wg.Go(func() error { return networkController.Run(ctx.Done()) })

	// start service controller
	wg.Go(func() error { return serviceController.Run(ctx.Done()) })

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	select {
	case <-term:
		glog.V(4).Info("Received SIGTERM, exiting gracefully...")
	case <-ctx.Done():
	}

	cancel()
	if err := wg.Wait(); err != nil {
		glog.Errorf("Unhandled error received: %v", err)
		return err
	}

	return nil
}

func initClients() (*kubernetes.Clientset, openstack.Interface, *extclientset.Clientset, error) {
	// Create kubernetes client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := util.NewClusterConfig(*kubeconfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create kubernetes clientset: %v", err)
	}
	kubeExtClient, err := extclientset.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create kubernetes apiextensions clientset: %v", err)
	}

	// Create OpenStack client from config file.
	osClient, err := openstack.NewClient(*cloudconfig, *kubeconfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could't initialize openstack client: %v", err)
	}

	return kubeClient, osClient, kubeExtClient, nil
}

func main() {
	util.InitFlags()
	util.InitLogs()
	defer util.FlushLogs()

	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	// Initilize kubernetes and openstack clients.
	kubeClient, osClient, kubeExtClient, err := initClients()
	if err != nil {
		glog.Fatal(err)
	}

	// Start stackube controllers.
	if err := startControllers(kubeClient, osClient, kubeExtClient); err != nil {
		glog.Fatal(err)
	}
}
