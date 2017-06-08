package rbacmanager

import (
	"fmt"
	"time"

	"git.openstack.org/openstack/stackube/pkg/auth-controller/k8sutil"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager/rbac"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/tenant"

	"github.com/go-kit/kit/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	kclient *kubernetes.Clientset
	logger  log.Logger

	nsInf cache.SharedIndexInformer

	queue workqueue.RateLimitingInterface
}

// New creates a new controller.
func New(conf tenant.Config, logger log.Logger) (*Controller, error) {
	cfg, err := k8sutil.NewClusterConfig(conf.Host, conf.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("init cluster config failed: %v", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init kubernetes client failed: %v", err)
	}

	o := &Controller{
		kclient: client,
		logger:  logger,
		queue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "rbacmanager"),
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

	errChan := make(chan error)
	go func() {
		v, err := c.kclient.Discovery().ServerVersion()
		if err != nil {
			errChan <- fmt.Errorf("communicating with server failed: %v", err)
			return
		}
		c.logger.Log("msg", "connection established", "cluster-version", v)
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
		c.logger.Log("msg", "TPR API endpoints ready")
	case <-stopc:
		return nil
	}

	go c.worker()

	go c.nsInf.Run(stopc)

	<-stopc
	return nil
}

func (c *Controller) keyFunc(obj interface{}) (string, bool) {
	k, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		c.logger.Log("msg", "creating key failed", "err", err)
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

	c.logger.Log("msg", "Namespace added", "key", key)
	c.enqueue(key)
}

func (c *Controller) handleNamespaceDelete(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}

	c.logger.Log("msg", "namespace deleted", "key", key)
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

	c.logger.Log("msg", "namespace updated", "key", key)
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

	c.logger.Log("msg", "syncrbac", "key", key)
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

	// create generate role and roleBinding for tenant
	role := rbac.GenerateRoleByNamespace(ns.Name)
	roleBinding := rbac.GenerateRoleBinding(ns.Name, tenant)

	// create role and rolebinding for tenant
	_, err := rbacClient.Roles(ns.Name).Create(role)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	c.logger.Log("msg", "Role(default-role) created", "namespace", ns.Name, "tenant", tenant)

	_, err = rbacClient.RoleBindings(ns.Name).Create(roleBinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	msg := fmt.Sprintf("RoleBindings(%s-rolebindings) created", tenant)
	c.logger.Log("msg", msg, "namespace", ns.Name, "tenant", tenant)

	return nil
}
