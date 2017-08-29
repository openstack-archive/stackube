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


function usage {
    echo "
Usage:
   bash $(basename $0) CONFIG_FILE
"
}

[ "$1" ] || { usage; exit 1; }
[ -f "$1" ] || { echo "Error: $1 not exists or not a file!"; exit 1; }

source $(readlink -f $1) || { echo "'source $(readlink -f $1)' failed!"; exit 1; }

[ "${CONTROL_NODE_PUBLIC_IP}" ] || { echo "Error: CONTROL_NODE_PUBLIC_IP not defined!"; exit 1; }
[ "${CONTROL_NODE_PRIVATE_IP}" ] || { echo "Error: CONTROL_NODE_PRIVATE_IP not defined!"; exit 1; }

[ "${NETWORK_NODES_PRIVATE_IP}" ] || { echo "Error: NETWORK_NODES_PRIVATE_IP not defined!"; exit 1; }
[ "${NETWORK_NODES_NEUTRON_EXT_IF}" ] || { echo "Error: NETWORK_NODES_NEUTRON_EXT_IF not defined!"; exit 1; }

[ "${COMPUTE_NODES_PRIVATE_IP}" ] || { echo "Error: COMPUTE_NODES_PRIVATE_IP not defined!"; exit 1; }

[ "${STORAGE_NODES_PRIVATE_IP}" ] || { echo "Error: STORAGE_NODES_PRIVATE_IP not defined!"; exit 1; }
[ "${STORAGE_NODES_CEPH_OSD_DATA_DIR}" ] || { echo "Error: STORAGE_NODES_CEPH_OSD_DATA_DIR not defined!"; exit 1; }

[ "${NEUTRON_PUBLIC_SUBNET}" ] || { echo "Error: NEUTRON_PUBLIC_SUBNET not defined!"; exit 1; }


#####################


function all_nodes_check_distro {
    for IP in $1; do
        ssh root@${IP} 'mkdir -p /tmp/stackube_install' 
        scp ${programDir}/{ensure_distro_supported.sh,lib_common.sh} root@${IP}:/tmp/stackube_install/
        ssh root@${IP} "/bin/bash /tmp/stackube_install/ensure_distro_supported.sh"
    done
}

function all_nodes_install_docker {
    for IP in $1; do
        ssh root@${IP} 'mkdir -p /tmp/stackube_install'
        scp ${programDir}/ensure_docker_installed.sh root@${IP}:/tmp/stackube_install/
        ssh root@${IP} "/bin/bash /tmp/stackube_install/ensure_docker_installed.sh"
    done
}


set -o errexit
set -o nounset
set -o pipefail
set -x


## log
logDir='/var/log/stackube'
logFile="${logDir}/install.log-$(date '+%Y-%m-%d_%H-%M-%S')"
mkdir -p ${logDir}

allIpList=`echo "
${CONTROL_NODE_PRIVATE_IP}
${NETWORK_NODES_PRIVATE_IP}
${COMPUTE_NODES_PRIVATE_IP}
${STORAGE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `

{
    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') all_nodes_check_distro"
    all_nodes_check_distro "${allIpList}"

    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') all_nodes_install_docker"
    all_nodes_install_docker "${allIpList}"

    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') deploy_ceph"
    bash ${programDir}/deploy_ceph.sh $(readlink -f $1)

    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') deploy_openstack"
    bash ${programDir}/deploy_openstack.sh $(readlink -f $1)

    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') deploy_kubernetes"
    bash ${programDir}/deploy_kubernetes.sh $(readlink -f $1)

    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') All done!"

    echo "
Additional information:
 * File /etc/stackube/openstack/admin-openrc.sh has been created. To use openstack command line tools you need to source the file.
 * File /etc/kubernetes/admin.conf has been created. To use kubectl you need to do 'export KUBECONFIG=/etc/kubernetes/admin.conf'.
 * The installation log file is available at: ${logFile}
"

} 2>&1 | tee -a ${logFile}




exit 0







