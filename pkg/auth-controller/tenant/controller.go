package tenant

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"git.openstack.org/openstack/stackube/pkg/apis/v1"
	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/client/auth"
	"git.openstack.org/openstack/stackube/pkg/auth-controller/rbacmanager/rbac"
	"git.openstack.org/openstack/stackube/pkg/openstack"
	"git.openstack.org/openstack/stackube/pkg/util"

	"github.com/golang/glog"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var (
	// NOTE: we should always use crv1.TenantResourcePlural.CRDGroup as CRD name
	crdTenant = crv1.TenantResourcePlural + "." + auth.CRDGroup

	resyncPeriod = 5 * time.Minute
)

// TenantController manages lify cycle of Tenant.
type TenantController struct {
	kclient   *kubernetes.Clientset
	crdclient *apiextensionsclient.Clientset
	tclient   *auth.AuthClient
	osclient  *openstack.Client
	tenInf    cache.SharedIndexInformer
	queue     workqueue.RateLimitingInterface
	config    Config
}

// Config defines configuration parameters for the TenantController.
type Config struct {
	KubeConfig  string
	CloudConfig string
}

// New creates a new tenant controller.
func New(conf Config) (*TenantController, error) {
	cfg, err := util.NewClusterConfig(conf.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("init cluster config failed: %v", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init kubernetes client failed: %v", err)
	}
	tclient, err := auth.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init restclient for tenant failed: %v", err)
	}
	crdclient, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init CRD client failed: %v", err)
	}

	openStackClient, err := openstack.NewClient(conf.CloudConfig)
	if err != nil {
		return nil, fmt.Errorf("init openstack client failed: %v", err)
	}

	c := &TenantController{
		crdclient: crdclient,
		kclient:   client,
		tclient:   tclient,
		osclient:  openStackClient,
		queue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tenant"),
		config:    conf,
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
		glog.V(4).Infof("Established connection established, cluster-version: %s", v)
		// Create CRD
		if _, err := c.createTenantCRD(c.crdclient); err != nil {
			if err != nil && !apierrors.IsAlreadyExists(err) {
				errChan <- fmt.Errorf("creating tenant CRD failed: %v", err)
			}
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
		glog.V(4).Info("CRD API endpoints ready")
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
		glog.V(4).Infof("Failed create key: %v", err)
		return k, false
	}
	return k, true
}

func (c *TenantController) handleAddTenant(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}
	glog.V(4).Infof("Added tenant %s", key)
	c.enqueue(key)
}

func (c *TenantController) handleDeleteTenant(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}
	glog.V(4).Infof("Deleted tenant %s", key)
	c.enqueue(key)
}

func (c *TenantController) handleUpdateTenant(old, cur interface{}) {
	key, ok := c.keyFunc(cur)
	if !ok {
		return
	}
	glog.V(4).Infof("Updated tenant %s", key)
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
		// Delete tenant related resources in k8s
		tenant := strings.Split(key, "/")
		deleteOptions := &apismetav1.DeleteOptions{
			TypeMeta: apismetav1.TypeMeta{
				Kind:       "ClusterRoleBinding",
				APIVersion: "rbac.authorization.k8s.io/v1beta1",
			},
		}
		err = c.kclient.Rbac().ClusterRoleBindings().Delete(tenant[1]+"-namespace-creater", deleteOptions)
		if err != nil && !apierrors.IsNotFound(err) {
			glog.Errorf("Failed delete ClusterRoleBinding for tenant %s: %v", tenant[1], err)
			return err
		}
		glog.V(4).Infof("Deleted ClusterRoleBinding %s", tenant[1])
		//Delete namespace
		err = c.deleteNamespace(tenant[1])
		if err != nil {
			return err
		}
		glog.V(4).Infof("Deleted namespace %s", tenant[1])
		// Delete all users on a tenant
		err = c.osclient.DeleteAllUsersOnTenant(tenant[1])
		if err != nil {
			glog.Errorf("Failed delete all users in the tenant %s: %v", tenant[1], err)
			return err
		}
		// Delete tenant in keystone
		err = c.osclient.DeleteTenant(tenant[1])
		if err != nil {
			glog.Errorf("Failed delete tenant %s: %v", tenant[1], err)
			return err
		}
		return nil
	}

	t := obj.(*v1.Tenant)
	glog.V(4).Infof("Sync tenant %s", key)
	err = c.syncTenant(t)
	if err != nil {
		return err
	}
	return nil
}

func (c *TenantController) createTenantCRD(clientset apiextensionsclient.Interface) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: apismetav1.ObjectMeta{
			Name: crdTenant,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   crv1.GroupName,
			Version: crv1.SchemeGroupVersion.Version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: crv1.TenantResourcePlural,
				Kind:   reflect.TypeOf(crv1.Tenant{}).Name(),
			},
		},
	}
	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err != nil {
		return nil, err
	}

	// wait for CRD being established
	if err = util.WaitForCRDReady(clientset, crdTenant); err != nil {
		return nil, err
	} else {
		return crd, nil
	}
}

func (c *TenantController) syncTenant(tenant *v1.Tenant) error {
	roleBinding := rbac.GenerateClusterRoleBindingByTenant(tenant.Name)
	_, err := c.kclient.Rbac().ClusterRoleBindings().Create(roleBinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create ClusterRoleBinding for tenant %s: %v", tenant.Name, err)
		return err
	}
	glog.V(4).Infof("Created ClusterRoleBindings %s-namespace-creater for tenant %s", tenant.Name, tenant.Name)
	if tenant.Spec.TenantID != "" {
		// Create user with the spec username and password in the given tenant
		err = c.osclient.CreateUser(tenant.Spec.UserName, tenant.Spec.Password, tenant.Spec.TenantID)
		if err != nil && !openstack.IsAlreadyExists(err) {
			glog.Errorf("Failed create user %s: %v", tenant.Spec.UserName, err)
			return err
		}
	} else {
		// Create tenant if the tenant not exist in keystone
		tenantID, err := c.osclient.CreateTenant(tenant.Name)
		if err != nil {
			return err
		}
		// Create user with the spec username and password in the created tenant
		err = c.osclient.CreateUser(tenant.Spec.UserName, tenant.Spec.Password, tenantID)
		if err != nil {
			return err
		}
	}

	// Create namespace which name is the same as the tenant's name
	err = c.createNamespace(tenant.Name)
	if err != nil {
		return err
	}
	glog.V(4).Infof("Created namespace %s for tenant %s", tenant.Name, tenant.Name)
	return nil
}

func (c *TenantController) createClusterRoles() error {
	nsCreater := rbac.GenerateClusterRole()
	_, err := c.kclient.Rbac().ClusterRoles().Create(nsCreater)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create ClusterRoles namespace-creater: %v", err)
		return err
	}
	glog.V(4).Info("Created ClusterRoles namespace-creater")
	return nil
}

func (c *TenantController) createNamespace(namespace string) error {
	_, err := c.kclient.CoreV1().Namespaces().Create(&apiv1.Namespace{
		ObjectMeta: apismetav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("Failed create namespace %s: %v", namespace, err)
		return err
	}
	return nil
}

func (c *TenantController) deleteNamespace(namespace string) error {
	err := c.kclient.CoreV1().Namespaces().Delete(namespace, apismetav1.NewDeleteOptions(0))
	if err != nil {
		glog.Errorf("Failed delete namespace %s: %v", namespace, err)
		return err
	}
	return nil
}
