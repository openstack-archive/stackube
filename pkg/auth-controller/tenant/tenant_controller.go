package tenant

import (
	"fmt"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	crdClient "git.openstack.org/openstack/stackube/pkg/auth-controller/client/auth"
	"git.openstack.org/openstack/stackube/pkg/openstack"

	"github.com/golang/glog"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// TenantController manages lify cycle of Tenant.
type TenantController struct {
	k8sClient       *kubernetes.Clientset
	tenantClient    *rest.RESTClient
	tenantScheme    *runtime.Scheme
	openstackClient *openstack.Client
}

// NewTenantController creates a new tenant controller.
func NewTenantController(kubeconfig, cloudconfig string) (*TenantController, error) {
	// Create OpenStack client from config
	openStackClient, err := openstack.NewClient(cloudconfig)
	if err != nil {
		return nil, fmt.Errorf("init openstack client failed: %v", err)
	}

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}
	clientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubeclient from config: %v", err)
	}

	// initialize CRD if it does not exist
	_, err = crdClient.CreateTenantCRD(clientset)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create CRD to kube-apiserver: %v", err)
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	// make a new config for our extension's API group, using the first config as a baseline
	tenantClient, tenantScheme, err := crdClient.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for CRD: %v", err)
	}

	c := &TenantController{
		tenantClient:    tenantClient,
		tenantScheme:    tenantScheme,
		k8sClient:       k8sClient,
		openstackClient: openStackClient,
	}

	if err = c.createClusterRoles(); err != nil {
		return nil, fmt.Errorf("failed to create cluster roles to kube-apiserver: %v", err)
	}

	return c, nil
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func (c *TenantController) GetTenantClient() *rest.RESTClient {
	return c.tenantClient
}

// Run the controller.
func (c *TenantController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	source := cache.NewListWatchFromClient(
		c.tenantClient,
		crv1.TenantResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())

	_, tenantInformor := cache.NewInformer(
		source,
		&crv1.Tenant{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		})

	go tenantInformor.Run(stopCh)
	<-stopCh
	return nil
}

func (c *TenantController) onAdd(obj interface{}) {
	tenant := obj.(*crv1.Tenant)
	glog.V(3).Infof("Tenant controller received new object %v\n", tenant)

	copyObj, err := c.tenantScheme.Copy(tenant)
	if err != nil {
		glog.Errorf("ERROR creating a deep copy of tenant object: %v\n", err)
		return
	}

	newTenant := copyObj.(*crv1.Tenant)
	c.syncTenant(newTenant)
}

func (c *TenantController) onUpdate(obj1, obj2 interface{}) {
	glog.Warning("tenant updates is not supported yet.")
}

func (c *TenantController) onDelete(obj interface{}) {
	tenant, ok := obj.(*crv1.Tenant)
	if !ok {
		return
	}

	glog.V(3).Infof("Tenant controller received deleted tenant %v\n", tenant)

	deleteOptions := &apismetav1.DeleteOptions{
		TypeMeta: apismetav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
	}
	tenantName := tenant.Name
	err := c.k8sClient.Rbac().ClusterRoleBindings().Delete(tenantName+"-namespace-creater", deleteOptions)
	if err != nil && !apierrors.IsNotFound(err) {
		glog.Errorf("Failed delete ClusterRoleBinding for tenant %s: %v", tenantName, err)
	} else {
		glog.V(4).Infof("Deleted ClusterRoleBinding %s", tenantName)
	}

	//Delete namespace
	err = c.deleteNamespace(tenantName)
	if err != nil {
		glog.Errorf("Delete namespace %s failed: %v", tenantName, err)
	} else {
		glog.V(4).Infof("Deleted namespace %s", tenantName)
	}

	// Delete all users on a tenant
	err = c.openstackClient.DeleteAllUsersOnTenant(tenantName)
	if err != nil {
		glog.Errorf("Failed delete all users in the tenant %s: %v", tenantName, err)
	}

	// Delete tenant in keystone
	if tenant.Spec.TenantID == "" {
		err = c.openstackClient.DeleteTenant(tenantName)
		if err != nil {
			glog.Errorf("Failed delete tenant %s: %v", tenantName, err)
		}
	}
}
