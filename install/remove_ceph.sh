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

#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x


source $(readlink -f $1)

[ "${CONTROL_NODE_PRIVATE_IP}" ]
[ "${STORAGE_NODES_PRIVATE_IP}" ]
[ "${STORAGE_NODES_CEPH_OSD_DATA_DIR}" ]


# ceph-mon
allIpList=`echo "
${CONTROL_NODE_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `

for IP in ${allIpList}; do
    ssh root@${IP} 'mkdir -p /tmp/stackube_install'
    scp ${programDir}/ceph/remove_ceph_from_node.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "/bin/bash /tmp/stackube_install/remove_ceph_from_node.sh"
done



# ceph-osd
storageIpList=(`echo "${STORAGE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g'`)
osdDataDirList=(`echo "${STORAGE_NODES_CEPH_OSD_DATA_DIR}" | sed -e 's/,/\n/g'`)
[ ${#storageIpList[@]} -eq ${#osdDataDirList[@]} ]

MAX=$((${#storageIpList[@]} - 1))
for i in `seq 0 ${MAX}`; do
    IP="${storageIpList[$i]}"
    dataDir="${osdDataDirList[$i]}"
    echo -e "\n------ ${IP} ${dataDir} ------"
    ssh root@${IP} 'mkdir -p /tmp/stackube_install'
    scp ${programDir}/ceph/remove_ceph_from_node.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "export CEPH_OSD_DATA_DIR='${dataDir}'
                    /bin/bash /tmp/stackube_install/remove_ceph_from_node.sh"
done



exit 0

