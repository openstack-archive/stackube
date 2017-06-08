package tenant

import (
	"fmt"
	"strings"
	"time"

	"git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/client/auth"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/k8sutil"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/openstack"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager/rbac"

	"github.com/go-kit/kit/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	extensionsobj "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	tprTenant = "tenant." + auth.TPRGroup

	resyncPeriod = 5 * time.Minute
)

// TenantController manages lify cycle of Tenant.
type TenantController struct {
	kclient  *kubernetes.Clientset
	tclient  *auth.AuthClient
	osclient *openstack.Client
	logger   log.Logger

	tenInf cache.SharedIndexInformer

	queue workqueue.RateLimitingInterface

	host   string
	config Config
}

// Config defines configuration parameters for the TenantController.
type Config struct {
	Host        string
	KubeConfig  string
	CloudConfig string
}

// New creates a new controller.
func New(conf Config, logger log.Logger) (*TenantController, error) {
	cfg, err := k8sutil.NewClusterConfig(conf.Host, conf.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("init cluster config failed: %v", err)
	}
	/*openStackClient, err := openstack.NewClient(conf.CloudConfig)
	if err != nil {
		return nil, fmt.Errorf("init openstack client failed: %v", err)
	}*/

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init kubernetes client failed: %v", err)
	}

	tclient, err := auth.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init restclient for tenant failed: %v", err)
	}

	c := &TenantController{
		kclient: client,
		tclient: tclient,
		//osclient: openStackClient,
		logger: logger,
		queue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tenant"),
		host:   cfg.Host,
		config: conf,
	}

	c.tenInf = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc:  tclient.Tenants(api.NamespaceAll).List,
			WatchFunc: tclient.Tenants(api.NamespaceAll).Watch,
		},
		&v1.Tenant{}, resyncPeriod, cache.Indexers{},
	)
	c.tenInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleAddTenant,
		DeleteFunc: c.handleDeleteTenant,
		UpdateFunc: c.handleUpdateTenant,
	})

	return c, nil
}

// Run the controller.
func (c *TenantController) Run(stopc <-chan struct{}) error {
	defer c.queue.ShutDown()

	errChan := make(chan error)
	go func() {
		v, err := c.kclient.Discovery().ServerVersion()
		if err != nil {
			errChan <- fmt.Errorf("communicating with server failed: %v", err)
			return
		}
		c.logger.Log("msg", "connection established", "cluster-version", v)

		if err := c.createTPRs(); err != nil {
			errChan <- fmt.Errorf("creating TPRs failed: %v", err)
			return
		}

		// Create clusterRole
		if err = c.createClusterRoles(); err != nil {
			errChan <- fmt.Errorf("creating clusterrole failed: %v", err)
			return
		}

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

	go c.tenInf.Run(stopc)

	<-stopc
	return nil
}

func (c *TenantController) keyFunc(obj interface{}) (string, bool) {
	k, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		c.logger.Log("msg", "creating key failed", "err", err)
		return k, false
	}
	return k, true
}

func (c *TenantController) handleAddTenant(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}

	c.logger.Log("msg", "Tenant added", "key", key)
	c.enqueue(key)
}

func (c *TenantController) handleDeleteTenant(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}

	c.logger.Log("msg", "Tenant deleted", "key", key)
	c.enqueue(key)
}

func (c *TenantController) handleUpdateTenant(old, cur interface{}) {
	key, ok := c.keyFunc(cur)
	if !ok {
		return
	}

	c.logger.Log("msg", "Tenant updated", "key", key)
	c.enqueue(key)
}

// enqueue adds a key to the queue. If obj is a key already it gets added directly.
// Otherwise, the key is extracted via keyFunc.
func (c *TenantController) enqueue(obj interface{}) {
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
func (c *TenantController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *TenantController) processNextWorkItem() bool {
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

func (c *TenantController) sync(key string) error {
	obj, exists, err := c.tenInf.GetIndexer().GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		//TODO: delete tenant and related resources
		tenant := strings.Split(key, "/")
		deleteOptions := &apimetav1.DeleteOptions{
			TypeMeta: apimetav1.TypeMeta{
				Kind:       "ClusterRoleBinding",
				APIVersion: "rbac.authorization.k8s.io/v1beta1",
			},
		}
		err = c.kclient.Rbac().ClusterRoleBindings().Delete(tenant[1]+"-namespace-creater", deleteOptions)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		c.logger.Log("msg", "delete tenant use cloud", "key", key)
		return nil
	}

	t := obj.(*v1.Tenant)

	c.logger.Log("msg", "sync tenant", "key", key)
	err = c.syncTenant(t)
	if err != nil {
		return err
	}

	return nil
}

func (c *TenantController) createTPRs() error {
	tprs := []*extensionsobj.ThirdPartyResource{
		{
			ObjectMeta: apimetav1.ObjectMeta{
				Name: tprTenant,
			},
			Versions: []extensionsobj.APIVersion{
				{Name: auth.TPRVersion},
			},
			Description: "Tpr for tenant",
		},
	}
	tprClient := c.kclient.Extensions().ThirdPartyResources()

	for _, tpr := range tprs {
		if _, err := tprClient.Create(tpr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		c.logger.Log("msg", "TPR created", "tpr", tpr.Name)
	}

	// We have to wait for the TPRs to be ready. Otherwise the initial watch may fail.
	err := k8sutil.WaitForTPRReady(c.kclient.CoreV1().RESTClient(), auth.TPRGroup, auth.TPRVersion, auth.TPRTenantName)
	if err != nil {
		return err
	}
	return nil
}

func (c *TenantController) syncTenant(tenant *v1.Tenant) error {
	roleBinding := rbac.GenerateClusterRoleBindingByTenant(tenant.Name)
	_, err := c.kclient.Rbac().ClusterRoleBindings().Create(roleBinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create ClusterRoleBinding for tenant: %v failed", tenant.Name)
	}
	// create tenant if the tenant not exist in keystone
	//err = osclient.CreateTenant(tenant.Name)
	//if err != nil {
	//}
	msg := fmt.Sprintf("ClusterRoleBindings(%s-namespace-creater) created", tenant.Name)
	c.logger.Log("msg", msg, "tenant", tenant.Name)

	//TODO: create namespace which name is the same as the tenant's name

	msg = fmt.Sprintf("namespace(%s) for tenant(%s) created", tenant.Name, tenant.Name)
	c.logger.Log("msg", msg)
	return nil
}

func (c *TenantController) createClusterRoles() error {
	nsCreater := rbac.GenerateClusterRole()
	_, err := c.kclient.Rbac().ClusterRoles().Create(nsCreater)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace creater clusterRole failed: %v", err)
	}
	c.logger.Log("msg", "ClusterRoles created", "ClusterRoles", "namespace-creater")
	return nil
}
