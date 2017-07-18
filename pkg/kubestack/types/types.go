package types

import (
	"net"

	"github.com/containernetworking/cni/pkg/types"
)

type NetConf struct {
	types.NetConf
	KubestackConfig  string `json:"kubestack-config"`
	KubernetesConfig string `json:"kubernetes-config"`
}

// K8sArgs is the valid CNI_ARGS used for Kubernetes
type K8sArgs struct {
	types.CommonArgs
	IP                         net.IP
	K8S_POD_NAME               types.UnmarshallableString
	K8S_POD_NAMESPACE          types.UnmarshallableString
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}
