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
# - ``CEPH_OSD_PUBLIC_IP``, ``CEPH_OSD_CLUSTER_IP``,
# - ``CEPH_OSD_DATA_DIR``   must be defined
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
sed -i "s/__PUBLIC_IP__/${CEPH_OSD_PUBLIC_IP}/g" /etc/stackube/ceph/ceph-osd/add_osd.sh
sed -i "s/__PUBLIC_IP__/${CEPH_OSD_PUBLIC_IP}/g" /etc/stackube/ceph/ceph-osd/config.json
sed -i "s/__CLUSTER_IP__/${CEPH_OSD_CLUSTER_IP}/g" /etc/stackube/ceph/ceph-osd/config.json


## bootstrap
mkdir -p ${CEPH_OSD_DATA_DIR}
docker run --net host  \
    --name stackube_ceph_bootstrap_osd  \
    -v /etc/stackube/ceph/ceph-osd/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/ceph:/var/log/kolla/:rw  \
    -v ${CEPH_OSD_DATA_DIR}:/var/lib/ceph/:rw  \
    \
    kolla/centos-binary-ceph-osd:4.0.0 /bin/bash /var/lib/kolla/config_files/add_osd.sh 

docker rm stackube_ceph_bootstrap_osd


## run
theOsd=`ls ${CEPH_OSD_DATA_DIR}/osd/ | grep -- 'ceph-' | head -n 1`
[ "${theOsd}" ]
osdId=`echo $theOsd | awk -F\- '{print $NF}'`
[ "${osdId}" ]

docker run -d  --net host  \
    --name stackube_ceph_osd_${osdId}  \
    -v /etc/stackube/ceph/ceph-osd/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/ceph:/var/log/kolla/:rw  \
    -v ${CEPH_OSD_DATA_DIR}:/var/lib/ceph/:rw  \
    \
    -e "KOLLA_SERVICE_NAME=ceph-osd"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    -e "OSD_ID=${osdId}"  \
    -e "JOURNAL_PARTITION=/var/lib/ceph/osd/ceph-${osdId}/journal" \
    \
    --restart unless-stopped \
    kolla/centos-binary-ceph-osd:4.0.0

sleep 5



exit 0
