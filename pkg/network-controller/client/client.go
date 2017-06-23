package client

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"

	tprv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
)

func NewClient(cfg *rest.Config) (*rest.RESTClient, *runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := tprv1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	config := *cfg
	config.GroupVersion = &tprv1.SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, nil, err
	}

	return client, scheme, nil
}
