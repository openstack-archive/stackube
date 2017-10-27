#!/bin/bash
# Copyright (c) 2017 OpenStack Foundation.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

STACKUBE_ROOT=$(dirname "${BASH_SOURCE}")

function install_docker {
    if is_ubuntu; then
        sudo apt-get update
        sudo apt-get install -y docker.io
    elif is_fedora; then
        sudo yum install -y docker
    else
        exit_distro_not_supported
    fi
 
    sudo systemctl start docker
}

function install_hyper {
    if is_ubuntu; then
        sudo apt-get update && sudo apt-get install -y qemu libvirt-bin
    elif is_fedora; then
        sudo yum install -y libvirt
    fi

    sudo systemctl restart libvirtd

    if command -v /usr/bin/hyperd > /dev/null 2>&1; then
        echo "hyperd already installed on this host, using it instead"
    else
        curl -sSL https://hypercontainer.io/install | sudo bash
    fi
    sudo sh -c 'cat>/etc/hyper/config <<EOF
Kernel=/var/lib/hyper/kernel
Initrd=/var/lib/hyper/hyper-initrd.img
Hypervisor=qemu
StorageDriver=overlay
gRPCHost=127.0.0.1:22318
EOF'
}

function install_frakti {
    if command -v /usr/bin/frakti > /dev/null 2>&1; then
        sudo rm -f /usr/bin/frakti
    fi
    sudo curl -sSL https://github.com/kubernetes/frakti/releases/download/${FRAKTI_VERSION}/frakti -o /usr/bin/frakti
    sudo chmod +x /usr/bin/frakti
    cgroup_driver=$(sudo docker info | awk '/Cgroup Driver/{print $3}')
    sudo sh -c "cat > /lib/systemd/system/frakti.service <<EOF
[Unit]
Description=Hypervisor-based container runtime for Kubernetes
Documentation=https://github.com/kubernetes/frakti
After=network.target
[Service]
ExecStart=/usr/bin/frakti --v=3 \
          --log-dir=/var/log/frakti \
          --logtostderr=false \
          --cgroup-driver=${cgroup_driver} \
          --listen=/var/run/frakti.sock \
          --hyper-endpoint=127.0.0.1:22318
MountFlags=shared
TasksMax=8192
LimitNOFILE=1048576
LimitNPROC=1048576
LimitCORE=infinity
TimeoutStartSec=0
Restart=on-abnormal
[Install]
WantedBy=multi-user.target
EOF"
}

function install_kubelet {
    if is_fedora; then
        sudo sh -c 'cat > /etc/yum.repos.d/kubernetes.repo <<EOF
[kubernetes]
name=Kubernetes
baseurl=http://yum.kubernetes.io/repos/kubernetes-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg
       https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
EOF'
        sudo setenforce 0
        sudo sed -i 's/SELINUX=enforcing/SELINUX=disabled/g' /etc/selinux/config
        sudo yum install -y kubelet-${KUBE_VERSION}-0 kubeadm-${KUBE_VERSION}-0 kubectl-${KUBE_VERSION}-0
    elif is_ubuntu; then
        sudo apt-get update && sudo apt-get install -y apt-transport-https
        sudo curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
        sudo sh -c 'cat > /etc/apt/sources.list.d/kubernetes.list <<EOF
deb http://apt.kubernetes.io/ kubernetes-xenial main
EOF'
        sudo apt-get update
        sudo apt-get install -y --allow-downgrades kubelet=${KUBE_VERSION}-00 kubeadm=${KUBE_VERSION}-00 kubectl=${KUBE_VERSION}-00
    else
        exit_distro_not_supported
    fi
}

function install_master {
    sed -i "s#KEYSTONE_HOST#${SERVICE_HOST}#g" ${STACKUBE_ROOT}/kubeadm.yaml
    sed -i "s#CLUSTER_CIDR#${CLUSTER_CIDR}#g" ${STACKUBE_ROOT}/kubeadm.yaml
    sudo kubeadm init --config ${STACKUBE_ROOT}/kubeadm.yaml
    # Enable schedule pods on the master for testing.
    sudo cp /etc/kubernetes/admin.conf $HOME/
    sudo chown $(id -u):$(id -g) $HOME/admin.conf
    export KUBECONFIG=$HOME/admin.conf
    kubectl taint nodes --all node-role.kubernetes.io/master-
}

function install_stackube_addons {
    # remove kube-dns deployment
    kubectl -n kube-system delete deployment kube-dns
    # remove kube-proxy daemonset
    kubectl -n kube-system delete daemonset kube-proxy

    source openrc admin admin
    public_network=$(openstack network list --long -f value | grep External | awk '{print $1}')

    keyring=`cat /etc/ceph/ceph.client.cinder.keyring  | grep 'key = ' | awk -F\ \=\  '{print $2}'`

    cat >stackube-configmap.yaml <<EOF
kind: ConfigMap
apiVersion: v1
metadata:
  name: stackube-config
  namespace: kube-system
data:
  auth-url: "https://${SERVICE_HOST}/identity_admin/v2.0"
  username: "admin"
  password: "${ADMIN_PASSWORD}"
  tenant-name: "admin"
  region: "RegionOne"
  ext-net-id: "${public_network}"
  plugin-name: "ovs"
  integration-bridge: "br-int"
  user-cidr: "${CLUSTER_CIDR}"
  user-gateway: "${CLUSTER_GATEWAY}"
  kubernetes-host: "${SERVICE_HOST}"
  kubernetes-port: "6443"
  keyring: "${keyring}" 
EOF
    kubectl create -f stackube-configmap.yaml
    kubectl create -f ${STACKUBE_ROOT}/../deployment/stackube.yaml
    kubectl create -f ${STACKUBE_ROOT}/../deployment/stackube-proxy.yaml
}

function install_flexvolume_plugin {
    kubectl create -f ${STACKUBE_ROOT}/../deployment/flexvolume/flexvolume-ds.yaml
}

function install_node {
    if [ "${KUBEADM_TOKEN}" = "" ]; then
        echo "KUBEADM_TOKEN must be set for node"
        exit 1
    fi
    sudo kubeadm join --token "${KUBEADM_TOKEN}" ${KUBERNETES_MASTER_IP}:${KUBERNETES_MASTER_PORT}
}

function configure_kubelet {
    if [ "${CONTAINER_RUNTIME}" = "frakti" ]; then
        sudo sed -i '2 i\Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --container-runtime-endpoint=/var/run/frakti.sock --feature-gates=AllAlpha=true"' /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
        sudo systemctl daemon-reload
    fi
}

function remove_kubernetes {
    sudo kubeadm reset
    sudo systemctl stop kubelet

    if is_fedora; then
        if [ "${CONTAINER_RUNTIME}" = "frakti" ]; then
            sudo yum remove -y qemu-hyper hyperstart hyper-container libvirt
        fi
        sudo yum remove -y kubelet kubeadm kubectl docker
    elif is_ubuntu; then
        if [ "${CONTAINER_RUNTIME}" = "frakti" ]; then
            sudo apt-get remove -y hyperstart hyper-container qemu libvirt-bin
        fi
        sudo apt-get remove -y kubelet kubeadm kubectl docker
    fi

    sudo rm -rf /usr/bin/frakti /etc/cni/net.d /lib/systemd/system/frakti.service
}

function install_stackube {
    if [ "${CONTAINER_RUNTIME}" != "frakti" ] && [ "${CONTAINER_RUNTIME}" != "docker" ]; then
        echo "Container runtime ${CONTAINER_RUNTIME} not supported"
        exit 1
    fi

    install_docker
    if [ "${CONTAINER_RUNTIME}" = "frakti" ]; then
        install_hyper
        install_frakti
    fi
    install_kubelet
}

function init_stackube {
    sudo systemctl daemon-reload
    sudo systemctl restart docker
    if [ "${CONTAINER_RUNTIME}" = "frakti" ]; then
        sudo systemctl restart libvirtd
        sudo systemctl restart hyperd
        sudo systemctl restart frakti
    fi

    if is_service_enabled kubernetes_master; then
        install_master
        install_stackube_addons
        install_flexvolume_plugin
        # approve kublelet's csr for the node.
        # TODO(harry) what if in multi-node cluster?
        kubectl certificate approve $(kubectl get csr | awk '/^csr/{print $1}')
    elif is_service_enabled kubernetes_node; then
        install_node
    fi
}

function configure_stackube {
    configure_kubelet
}

# check for service enabled
if is_service_enabled stackube; then

    if [[ "$1" == "stack" && "$2" == "install" ]]; then
        echo_summary "Installing stackube"
        install_stackube

    elif [[ "$1" == "stack" && "$2" == "post-config" ]]; then
        echo_summary "Configuring stackube"
        configure_stackube

    elif [[ "$1" == "stack" && "$2" == "extra" ]]; then
        # Initialize and start the stackube service
        echo_summary "Initializing stackube"
        init_stackube
    fi

    if [[ "$1" == "unstack" ]]; then
        remove_kubernetes
    fi

    if [[ "$1" == "clean" ]]; then
        echo ''
    fi
fi
