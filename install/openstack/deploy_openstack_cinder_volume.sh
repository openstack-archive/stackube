#!/bin/bash
#
# Dependencies:
#
# - ``API_IP``, ``RABBITMQ_PWD``
# - ``KEYSTONE_ADMIN_PWD``
# - ``KEYSTONE_CINDER_PWD``, ``MYSQL_CINDER_PWD``must be defined
#


programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x


## log dir
mkdir -p /var/log/stackube/openstack
chmod 777 /var/log/stackube/openstack


## start_container - cinder-volume
docker run -d  --net host  \
    --name stackube_openstack_cinder_volume  \
    -v /etc/stackube/openstack/cinder-volume/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -v /run/:/run/:shared  \
    -v /dev/:/dev/:rw  \
    \
    -e "KOLLA_SERVICE_NAME=cinder-volume"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    --privileged  \
    kolla/centos-binary-cinder-volume:4.0.0

sleep 5



exit 0
