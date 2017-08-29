#!/bin/bash
#


programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -x


## remove docker containers
stackubeCephConstaners=`docker ps -a | awk '{print $NF}' | grep '^stackube_ceph_' `
if [ "${stackubeCephConstaners}" ]; then
    docker rm -f $stackubeCephConstaners || exit 1
fi

## rm dirs
rm -fr /etc/stackube/ceph  /var/log/stackube/ceph  /var/lib/stackube/ceph  ${CEPH_OSD_DATA_DIR} || exit 1



exit 0

