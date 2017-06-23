package client

import (
	"time"

	tprv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/rest"
)

func CreateNetworkTPR(clientset kubernetes.Interface) error {
	tpr := &v1beta1.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "network." + tprv1.GroupName,
		},
		Versions: []v1beta1.APIVersion{
			{Name: tprv1.SchemeGroupVersion.Version},
		},
		Description: "An Network ThirdPartyResource",
	}
	_, err := clientset.ExtensionsV1beta1().ThirdPartyResources().Create(tpr)
	return err
}

func WaitForNetworkResource(networkClient *rest.RESTClient) error {
	return wait.Poll(100*time.Millisecond, 60*time.Second, func() (bool, error) {
		_, err := networkClient.Get().Namespace(apiv1.NamespaceDefault).Resource(tprv1.NetworkResourcePlural).DoRaw()
		if err == nil {
			return true, nil
		}
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	})
}

func WaitForNetworkInstanceProcessed(networkClient *rest.RESTClient, name string) error {
	return wait.Poll(100*time.Millisecond, 10*time.Second, func() (bool, error) {
		var network tprv1.Network
		err := networkClient.Get().
			Resource(tprv1.NetworkResourcePlural).
			Namespace(apiv1.NamespaceDefault).
			Name(name).
			Do().Into(&network)

		if err == nil && network.Status.State == tprv1.NetworkActive {
			return true, nil
		}

		return false, err
	})
}
