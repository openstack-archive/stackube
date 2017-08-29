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
# Dependencies:
#
# - ``CEPH_MON_PUBLIC_IP``
# - ``CEPH_FSID``  must be defined
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
mkdir -p /var/log/stackube/ceph
chmod 777 /var/log/stackube/ceph


## config files
mkdir -p /etc/stackube/ceph
cp -a ${programDir}/config_ceph/ceph-mon /etc/stackube/ceph/
sed -i "s/__FSID__/${CEPH_FSID}/g" /etc/stackube/ceph/ceph-mon/ceph.conf
sed -i "s/__PUBLIC_IP__/${CEPH_MON_PUBLIC_IP}/g" /etc/stackube/ceph/ceph-mon/ceph.conf
sed -i "s/__PUBLIC_IP__/${CEPH_MON_PUBLIC_IP}/g" /etc/stackube/ceph/ceph-mon/config.json


mkdir -p /var/lib/stackube/ceph/ceph_mon_config  && \
mkdir -p /var/lib/stackube/ceph/ceph_mon  && \
docker run --net host  \
    --name stackube_ceph_bootstrap_mon  \
    -v /etc/stackube/ceph/ceph-mon/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/ceph:/var/log/kolla/:rw  \
    -v /var/lib/stackube/ceph/ceph_mon_config:/etc/ceph/:rw  \
    -v /var/lib/stackube/ceph/ceph_mon:/var/lib/ceph/:rw  \
    \
    -e "KOLLA_BOOTSTRAP="  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    -e "MON_IP=${CEPH_MON_PUBLIC_IP}" \
    -e "HOSTNAME=${CEPH_MON_PUBLIC_IP}" \
    kolla/centos-binary-ceph-mon:4.0.0

docker rm stackube_ceph_bootstrap_mon


docker run -d  --net host  \
    --name stackube_ceph_mon  \
    -v /etc/stackube/ceph/ceph-mon/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/ceph:/var/log/kolla/:rw  \
    -v /var/lib/stackube/ceph/ceph_mon_config:/etc/ceph/:rw  \
    -v /var/lib/stackube/ceph/ceph_mon:/var/lib/ceph/:rw  \
    \
    -e "KOLLA_SERVICE_NAME=ceph-mon"  \
    -e "HOSTNAME=${CEPH_MON_PUBLIC_IP}"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    kolla/centos-binary-ceph-mon:4.0.0

sleep 5

docker exec stackube_ceph_mon ceph -s



exit 0
