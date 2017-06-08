package k8sutil

import (
	"fmt"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// WaitForTPRReady waits for a third party resource to be available
// for use.
func WaitForTPRReady(restClient rest.Interface, tprGroup, tprVersion, tprName string) error {
	return wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
		res := restClient.Get().AbsPath("apis", tprGroup, tprVersion, tprName).Do()
		err := res.Error()
		if err != nil {
			// RESTClient returns *apierrors.StatusError for any status codes < 200 or > 206
			// and http.Client.Do errors are returned directly.
			if se, ok := err.(*apierrors.StatusError); ok {
				if se.Status().Code == http.StatusNotFound {
					return false, nil
				}
			}
			return false, err
		}

		var statusCode int
		res.StatusCode(&statusCode)
		if statusCode != http.StatusOK {
			return false, fmt.Errorf("invalid status code: %d", statusCode)
		}

		return true, nil
	})
}

func NewClusterConfig(host, kubeConfig string) (*rest.Config, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(host, kubeConfig)
	if err != nil {
		return nil, err
	}
	cfg.QPS = 100
	cfg.Burst = 100

	return cfg, nil
}
