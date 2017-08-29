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


[ "${CONTROL_NODE_PRIVATE_IP}" ] || { echo "Error: CONTROL_NODE_PRIVATE_IP not defined!"; exit 1; }
[ "${NETWORK_NODES_PRIVATE_IP}" ] || { echo "Error: NETWORK_NODES_PRIVATE_IP not defined!"; exit 1; }
[ "${COMPUTE_NODES_PRIVATE_IP}" ] || { echo "Error: COMPUTE_NODES_PRIVATE_IP not defined!"; exit 1; }
[ "${STORAGE_NODES_PRIVATE_IP}" ] || { echo "Error: STORAGE_NODES_PRIVATE_IP not defined!"; exit 1; }
[ "${STORAGE_NODES_CEPH_OSD_DATA_DIR}" ] || { echo "Error: STORAGE_NODES_CEPH_OSD_DATA_DIR not defined!"; exit 1; }


#####################

set -x


## log
logDir='/var/log/stackube'
logFile="${logDir}/remove.log-$(date '+%Y-%m-%d_%H-%M-%S')"
mkdir -p ${logDir}

allIpList=`echo "
${CONTROL_NODE_PRIVATE_IP}
${NETWORK_NODES_PRIVATE_IP}
${COMPUTE_NODES_PRIVATE_IP}
${STORAGE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `

{
    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') remove_kubernetes"
    remove_kubernetes=''
    for i in `seq 1 10`; do
        bash ${programDir}/remove_kubernetes.sh $(readlink -f $1)
        if [ "$?" == "0" ]; then
            remove_kubernetes='done'
            break
        fi
    done
    [ "${remove_kubernetes}" == "done" ] || { echo "Error: remove_kubernetes failed !"; exit 1;  }

    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') remove_openstack"
    remove_openstack=''
    for i in `seq 1 10`; do
        bash ${programDir}/remove_openstack.sh $(readlink -f $1)
        if [ "$?" == "0" ]; then
            remove_openstack='done'
            break
        fi
    done
    [ "${remove_openstack}" == "done" ] || { echo "Error: remove_openstack failed !"; exit 1;  }

    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') remove_ceph"
    remove_ceph=''
    for i in `seq 1 10`; do
        bash ${programDir}/remove_ceph.sh $(readlink -f $1)
        if [ "$?" == "0" ]; then
            remove_ceph='done'
            break
        fi
    done
    [ "${remove_ceph}" == "done" ] || { echo "Error: remove_ceph failed !"; exit 1;  }

    echo -e "\n$(date '+%Y-%m-%d %H:%M:%S') All done!"

} 2>&1 | tee -a ${logFile}


allStats=(${PIPESTATUS[@]})
if [ "${allStats[0]}" != "0" ]; then
    exit 1
fi


exit 0




