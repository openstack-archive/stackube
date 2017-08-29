#!/bin/bash
#
# Dependencies:
#
# - ``MYSQL_ROOT_PWD`` must be defined
#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x


## mariadb
mkdir -p /var/lib/stackube/openstack/mariadb  && \
docker run -d \
    --name stackube_openstack_mariadb \
    --net host  \
    -e MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PWD} \
    -v /var/lib/stackube/openstack/mariadb:/var/lib/mysql \
    --restart unless-stopped \
    mariadb:5.5

sleep 5

exit 0

