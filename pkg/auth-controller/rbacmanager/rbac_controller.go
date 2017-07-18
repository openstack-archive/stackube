package rbacmanager

import (
	"fmt"
	"time"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager/rbac"
	crdClient "git.openstack.org/openstack/stackube/pkg/kubecrd"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	resyncPeriod = 5 * time.Minute
)

type Controller struct {
	kclient       *kubernetes.Clientset
	nsInf         cache.SharedIndexInformer
	queue         workqueue.RateLimitingInterface
	kubeCRDClient *crdClient.CRDClient
	systemCIDR    string
	systemGateway string
}

// New creates a new RBAC controller.
func New(kubeconfig string,
	kubeCRDClient *crdClient.CRDClient,
	systemCIDR string,
	systemGateway string,
) (*Controller, error) {
	cfg, err := util.NewClusterConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("init cluster config failed: %v", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init kubernetes client failed: %v", err)
	}

	o := &Controller{
		kclient:       client,
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "rbacmanager"),
		kubeCRDClient: kubeCRDClient,
		systemCIDR:    systemCIDR,
		systemGateway: systemGateway,
	}

	o.nsInf = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(o.kclient.Core().RESTClient(), "namespaces", api.NamespaceAll, nil),
		&v1.Namespace{}, resyncPeriod, cache.Indexers{},
	)

	o.nsInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    o.handleNamespaceAdd,
		DeleteFunc: o.handleNamespaceDelete,
		UpdateFunc: o.handleNamespaceUpdate,
	})

	return o, nil
}

// Run the controller.
func (c *Controller) Run(stopc <-chan struct{}) error {
	defer c.queue.ShutDown()

	glog.V(4).Info("Starting rbac manager")
	go c.worker()
	go c.nsInf.Run(stopc)

	<-stopc
	return nil
}

func (c *Controller) keyFunc(obj interface{}) (string, bool) {
	k, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		glog.Errorf("Creating key failed: %v", err)
		return k, false
	}
	return k, true
}

// enqueue adds a key to the queue. If obj is a key already it gets added directly.
// Otherwise, the key is extracted via keyFunc.
func (c *Controller) enqueue(obj interface{}) {
	if obj == nil {
		return
	}

	key, ok := obj.(string)
	if !ok {
		key, ok = c.keyFunc(obj)
		if !ok {
			return
		}
	}

	c.queue.Add(key)
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *Controller) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync(key.(string))
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("Sync %q failed: %v", key, err))
	c.queue.AddRateLimited(key)

	return true
}

func (c *Controller) handleNamespaceAdd(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}
	// check if this is a system reserved namespace
	if util.IsSystemNamespace(key) {
		if err := c.initSystemReservedTenantNetwork(); err != nil {
			glog.Error(err)
			return
		}
	}
	glog.V(4).Infof("Added namespace %s", key)
	c.enqueue(key)
}

func (c *Controller) initSystemReservedTenantNetwork() error {
	tenant := &crv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.SystemTenant,
			Namespace: util.SystemTenant,
		},
		Spec: crv1.TenantSpec{
			UserName: util.SystemTenant,
			Password: util.SystemPassword,
		},
	}

	if err := c.kubeCRDClient.AddTenant(tenant); err != nil {
		return err
	}

	// NOTE(harry): we do not support update Network, so although configurable,
	// user can not update CIDR by changing the configuration, unless manually delete
	// that system network. We may need to document this.
	network := &crv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.SystemNetwork,
			Namespace: util.SystemTenant,
		},
		Spec: crv1.NetworkSpec{
			CIDR:    c.systemCIDR,
			Gateway: c.systemGateway,
		},
	}

	// network controller will always check if Tenant is ready so we will not wait here
	if err := c.kubeCRDClient.AddNetwork(network); err != nil {
		return err
	}

	return nil
}

func (c *Controller) handleNamespaceDelete(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}
	glog.V(4).Infof("Deleted namespace %s", key)
	c.enqueue(key)
}

func (c *Controller) handleNamespaceUpdate(old, cur interface{}) {
	oldns := old.(*v1.Namespace)
	curns := cur.(*v1.Namespace)
	if oldns.ResourceVersion == curns.ResourceVersion {
		return
	}
	key, ok := c.keyFunc(cur)
	if !ok {
		return
	}
	glog.V(4).Infof("Updated namespace %s", key)
	c.enqueue(key)
}

func (c *Controller) sync(key string) error {
	obj, exists, err := c.nsInf.GetIndexer().GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	ns := obj.(*v1.Namespace)
	glog.V(4).Infof("Sync RBAC %s", key)
	err = c.syncRbac(ns)
	if err != nil {
		return err
	}

	return nil
}

func (c *Controller) syncRbac(ns *v1.Namespace) error {
	if ns.DeletionTimestamp != nil {
		return nil
	}
	tenant, ok := ns.Labels["tenant"]
	if !ok {
		return nil
	}
	rbacClient := c.kclient.Rbac()
	// Create role for tenant
	role := rbac.GenerateRoleByNamespace(ns.Name)
	_, err := rbacClient.Roles(ns.Name).Create(role)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create default-role in namespace %s for tenant %s: %v", ns.Name, tenant, err)
		return err
	}
	glog.V(4).Infof("Created default-role in namespace %s for tenant %s", ns.Name, tenant)
	// Create rolebinding for tenant
	roleBinding := rbac.GenerateRoleBinding(ns.Name, tenant)
	_, err = rbacClient.RoleBindings(ns.Name).Create(roleBinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create %s-rolebindings in namespace %s for tenant %s: %v", tenant, ns.Name, tenant, err)
		return err
	}
	glog.V(4).Infof("Created %s-rolebindings in namespace %s for tenant %s", tenant, ns.Name, tenant)
	return nil
}
