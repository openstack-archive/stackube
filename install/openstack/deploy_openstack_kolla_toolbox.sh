#!/bin/bash
#
# Dependencies:
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


## kolla-toolbox
docker run -d  --net host  \
    --name stackube_openstack_kolla_toolbox  \
    -v /run/:/run/:shared  \
    -v /dev/:/dev/:rw  \
    -v /etc/stackube/openstack/kolla-toolbox/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -e "KOLLA_SERVICE_NAME=kolla-toolbox"  \
    -e "ANSIBLE_LIBRARY=/usr/share/ansible"  \
    -e "ANSIBLE_NOCOLOR=1"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    --restart unless-stopped  \
    --privileged  \
    kolla/centos-binary-kolla-toolbox:4.0.0

sleep 5


exit 0

