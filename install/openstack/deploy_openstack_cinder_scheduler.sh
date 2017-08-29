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


## start_container - cinder-scheduler
docker run -d  --net host  \
    --name stackube_openstack_cinder_scheduler  \
    -v /etc/stackube/openstack/cinder-scheduler/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    \
    -e "KOLLA_SERVICE_NAME=cinder-scheduler"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    kolla/centos-binary-cinder-scheduler:4.0.0

sleep 5



exit 0
