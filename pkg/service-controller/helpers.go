package service

import (
	"fmt"

	"k8s.io/api/core/v1"
)

const (
	lbPrefix = "stackube"
)

func buildServiceName(service *v1.Service) string {
	return fmt.Sprintf("%s_%s", service.Namespace, service.Name)
}

func buildLoadBalancerName(service *v1.Service) string {
	return fmt.Sprintf("%s_%s_%s", lbPrefix, service.Namespace, service.Name)
}
