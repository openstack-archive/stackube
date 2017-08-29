#!/bin/bash
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

