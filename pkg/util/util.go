package util

import (
	"errors"

	apiv1 "k8s.io/api/core/v1"
)

const (
	namePrefix = "kube"

	SystemTenant   = apiv1.NamespaceDefault
	SystemPassword = "password"

	SystemNetwork = apiv1.NamespaceDefault
)

var ErrNotFound = errors.New("NotFound")
var ErrMultipleResults = errors.New("MultipleResults")

func BuildNetworkName(namespace, name string) string {
	if IsSystemNamespace(namespace) {
		// All system namespaces shares same network.
		return namePrefix + "-" + SystemTenant + "-" + SystemTenant
	}
	return namePrefix + "-" + namespace + "-" + name
}

func BuildLoadBalancerName(namespace, name string) string {
	if IsSystemNamespace(namespace) {
		namespace = SystemTenant
	}
	return namePrefix + "-" + namespace + "-" + name
}

func BuildPortName(namespace, podName string) string {
	if IsSystemNamespace(namespace) {
		namespace = SystemTenant
	}
	return namePrefix + "-" + namespace + "-" + podName
}

func IsSystemNamespace(ns string) bool {
	switch ns {
	case
		"default",
		"kube-system",
		"kube-public":
		return true
	}
	return false
}
