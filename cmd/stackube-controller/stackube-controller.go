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
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
)

var (
	kubeconfig = pflag.String("kubeconfig", "/etc/kubernetes/admin.conf",
		"path to kubernetes admin config file")
	cloudconfig = pflag.String("cloudconfig", "/etc/stackube.conf",
		"path to stackube config file")
)

func startControllers(cfg tenant.Config) error {
	// Creates a new tenant controller
	tc, err := tenant.New(cfg)
	if err != nil {
		return err
	}

	// Creates a new RBAC controller
	rm, err := rbacmanager.New(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg, ctx := errgroup.WithContext(ctx)

	// start auth controllers in stackube
	wg.Go(func() error { return tc.Run(ctx.Done()) })
	wg.Go(func() error { return rm.Run(ctx.Done()) })

	networkController, err := network.NewNetworkController(
		cfg.KubeConfig,
		cfg.CloudConfig,
	)
	if err != nil {
		return err
	}

	// start network controller
	wg.Go(func() error { return networkController.Run(ctx.Done()) })

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

func verifyClientSetting() error {
	config, err := util.NewClusterConfig(*kubeconfig)
	if err != nil {
		return fmt.Errorf("Init kubernetes cluster failed: %v", err)
	}

	_, err = kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("Init kubernetes clientset failed: %v", err)
	}

	_, err = openstack.NewClient(*cloudconfig)
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

	// Start stackube controllers.
	cfg := tenant.Config{
		KubeConfig:  *kubeconfig,
		CloudConfig: *cloudconfig,
	}
	if err := startControllers(cfg); err != nil {
		glog.Fatal(err)
	}
}
