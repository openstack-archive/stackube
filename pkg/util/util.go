/*
Copyright (c) 2017 OpenStack Foundation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
