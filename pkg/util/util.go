package util

import (
	"errors"
)

const (
	namePrefix = "kube"
)

var ErrNotFound = errors.New("NotFound")
var ErrMultipleResults = errors.New("MultipleResults")

func BuildNetworkName(namespace, name string) string {
	return namePrefix + "_" + namespace + "_" + name
}

func BuildLoadBalancerName(namespace, name string) string {
	return namePrefix + "_" + namespace + "_" + name
}
