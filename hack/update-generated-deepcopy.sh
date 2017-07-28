#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

STACKUBE_ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${STACKUBE_ROOT}"

go get k8s.io/kubernetes/vendor/k8s.io/kube-gen/cmd/deepcopy-gen

deepcopy-gen -i ./pkg/apis/v1

