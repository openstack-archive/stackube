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
[ "${NETWORK_NODES_PRIVATE_IP}" ]
[ "${COMPUTE_NODES_PRIVATE_IP}" ]
[ "${STORAGE_NODES_PRIVATE_IP}" ]



allIpList=`echo "
${CONTROL_NODE_PRIVATE_IP}
${NETWORK_NODES_PRIVATE_IP}
${COMPUTE_NODES_PRIVATE_IP}
${STORAGE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `

for IP in ${allIpList}; do
    ssh root@${IP} 'mkdir -p /tmp/stackube_install'
    scp ${programDir}/openstack/remove_openstack_from_node.sh root@${IP}:/tmp/stackube_install/
    scp ${programDir}/lib_tls.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "/bin/bash /tmp/stackube_install/remove_openstack_from_node.sh"
done



exit 0

