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


programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x


source $(readlink -f $1)

[ "${CONTROL_NODE_PUBLIC_IP}" ]
[ "${CONTROL_NODE_PRIVATE_IP}" ]
[ "${NETWORK_NODES_PRIVATE_IP}" ]
[ "${COMPUTE_NODES_PRIVATE_IP}" ]


export KUBERNETES_API_PUBLIC_IP="${CONTROL_NODE_PUBLIC_IP}"
export KUBERNETES_API_PRIVATE_IP="${CONTROL_NODE_PRIVATE_IP}"
export KEYSTONE_URL="https://${CONTROL_NODE_PRIVATE_IP}:5001/v2.0"
export KEYSTONE_ADMIN_URL="https://${CONTROL_NODE_PRIVATE_IP}:35358/v2.0"
export CLUSTER_CIDR="10.244.0.0/16"
export CLUSTER_GATEWAY="10.244.0.1"
export CONTAINER_CIDR="10.244.1.0/24"
export FRAKTI_VERSION="v1.0"


########## control & compute nodes ##########

allIpList=`echo "
${CONTROL_NODE_PRIVATE_IP}
${COMPUTE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `

# hyperd frakti
for IP in ${allIpList}; do
    ssh root@${IP} 'mkdir -p /tmp/stackube_install'
    scp ${programDir}/kubernetes/deploy_hyperd_frakti.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "export FRAKTI_VERSION='${FRAKTI_VERSION}'
                    export STREAMING_SERVER_ADDR='${IP}'
                    /bin/bash /tmp/stackube_install/deploy_hyperd_frakti.sh"
done

# kubeadm kubectl kubelet
for IP in ${allIpList}; do
    ssh root@${IP} 'mkdir -p /tmp/stackube_install'
    scp ${programDir}/kubernetes/deploy_kubeadm_kubectl_kubelet.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "/bin/bash /tmp/stackube_install/deploy_kubeadm_kubectl_kubelet.sh"
done



########## control node ##########

# kubernetes master
sed -i "s|__KEYSTONE_URL__|${KEYSTONE_URL}|g" ${programDir}/kubernetes/kubeadm.yaml
sed -i "s|__POD_NET_CIDR__|${CLUSTER_CIDR}|g" ${programDir}/kubernetes/kubeadm.yaml
sed -i "s/__KUBERNETES_API_PUBLIC_IP__/${KUBERNETES_API_PUBLIC_IP}/g" ${programDir}/kubernetes/kubeadm.yaml
sed -i "s/__KUBERNETES_API_PRIVATE_IP__/${KUBERNETES_API_PRIVATE_IP}/g" ${programDir}/kubernetes/kubeadm.yaml
/bin/bash ${programDir}/kubernetes/deploy_kubernetes_init_master.sh
sleep 3



export KUBECONFIG=/etc/kubernetes/admin.conf


# install stackube addons
/bin/bash ${programDir}/kubernetes/deploy_kubernetes_install_stackube_addons.sh
sleep 10


# add nodes
KUBEADM_TOKEN=`kubeadm token list | grep 'kubeadm init' | head -1 | awk '{print $1}'`
allIpList=`echo "
${COMPUTE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq | grep -v "${CONTROL_NODE_PRIVATE_IP}"`
for IP in ${allIpList}; do
    ssh root@${IP} "kubeadm join --token ${KUBEADM_TOKEN} ${CONTROL_NODE_PRIVATE_IP}:6443"
done


# Enable schedule pods on the master (control node) if it's also designated as a compute node
set +e
check=`echo "
${COMPUTE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq | grep "${CONTROL_NODE_PRIVATE_IP}" `
if [ "${check}" ]; then
    kubectl taint nodes $(hostname) node-role.kubernetes.io/master-
fi
set -e


# certificate approve
sleep 5
/bin/bash ${programDir}/kubernetes/deploy_kubernetes_certificate_approve.sh



## check
sleep 3
kubectl get nodes
kubectl get csr --all-namespaces




########## control (k8s master) & compute nodes ###########

allIpList=`echo "
${CONTROL_NODE_PRIVATE_IP}
${COMPUTE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `

# install ovs for cni
for IP in ${allIpList}; do
    ssh root@${IP} "yum install centos-release-openstack-ocata.noarch -y"
    ssh root@${IP} "yum install openvswitch -y"
done

# install ceph for kubelet
for IP in ${allIpList}; do
    ssh root@${IP} "yum install centos-release-openstack-ocata.noarch -y"
    ssh root@${IP} "yum install ceph -y"
    ssh root@${IP} "systemctl disable ceph.target ceph-mds.target ceph-mon.target ceph-osd.target"
    scp -r /var/lib/stackube/ceph/ceph_mon_config/*  root@${IP}:/etc/ceph/
    ssh root@${IP} "ceph -s"
    ssh root@${IP} "rbd -p cinder --id cinder --keyring=/etc/ceph/ceph.client.cinder.keyring ls"
done




exit 0
