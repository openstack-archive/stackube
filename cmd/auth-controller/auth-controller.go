package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"git.openstack.org/openstack/stackube/pkg/auth-controller/k8sutil"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/openstack"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/tenant"

	"github.com/golang/glog"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
)

var (
	cfg tenant.Config
)

func init() {
	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	flagset.StringVar(&cfg.KubeConfig, "kubeconfig", "", "- path to kubeconfig")
	flagset.StringVar(&cfg.CloudConfig, "cloudconfig", "", "- path to cloudconfig")

	flagset.Parse(os.Args[1:])
}

func Main() int {
	// Verify client setting at the beginning and fail early if there are errors.
	err := verifyClientSetting()
	if err != nil {
		glog.Error(err)
		return 1
	}
	// Creates a new tenant controller
	tc, err := tenant.New(cfg)
	if err != nil {
		glog.Error(err)
		return 1
	}
	// Creates a new RBAC controller
	rm, err := rbacmanager.New(cfg)
	if err != nil {
		glog.Error(err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg, ctx := errgroup.WithContext(ctx)

	wg.Go(func() error { return tc.Run(ctx.Done()) })
	wg.Go(func() error { return rm.Run(ctx.Done()) })

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
		return 1
	}

	return 0
}

func verifyClientSetting() error {
	config, err := k8sutil.NewClusterConfig(cfg.KubeConfig)
	if err != nil {
		return fmt.Errorf("Init cluster config failed: %v", err)
	}
	_, err = kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("Init kubernetes clientset failed: %v", err)
	}
	_, err = openstack.NewClient(cfg.CloudConfig)
	if err != nil {
		return fmt.Errorf("Init openstack client failed: %v", err)
	}
	return nil
}

func main() {
	os.Exit(Main())
}
