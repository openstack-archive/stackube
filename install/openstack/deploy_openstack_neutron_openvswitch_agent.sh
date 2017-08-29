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
# - ``OVSDB_IP``
# - ``ML2_LOCAL_IP`` must be defined
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


## openvswitch-db-server
sed -i "s/__OVSDB_IP__/${OVSDB_IP}/g" /etc/stackube/openstack/openvswitch-db-server/config.json
mkdir -p /var/lib/stackube/openstack/openvswitch
docker run -d  --net host  \
    --name stackube_openstack_openvswitch_db  \
    -v /etc/stackube/openstack/openvswitch-db-server/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -v /var/lib/stackube/openstack/openvswitch/:/var/lib/openvswitch/:rw  \
    -v /run:/run:shared  \
    \
    -e "KOLLA_SERVICE_NAME=openvswitch-db"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    kolla/centos-binary-openvswitch-db-server:4.0.0

sleep 5

# config br
docker exec stackube_openstack_openvswitch_db /usr/local/bin/kolla_ensure_openvswitch_configured br-ex


## openvswitch-vswitchd
docker run -d  --net host  \
    --name stackube_openstack_openvswitch_vswitchd  \
    -v /etc/stackube/openstack/openvswitch-vswitchd/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -v /run:/run:shared  \
    -v /lib/modules:/lib/modules:ro  \
    \
    -e "KOLLA_SERVICE_NAME=openvswitch-vswitchd"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    --privileged  \
    kolla/centos-binary-openvswitch-vswitchd:4.0.0

sleep 5


## start_container - neutron-openvswitch-agent
sed -i "s/__OVSDB_IP__/${OVSDB_IP}/g" /etc/stackube/openstack/neutron-openvswitch-agent/ml2_conf.ini
sed -i "s/__LOCAL_IP__/${ML2_LOCAL_IP}/g" /etc/stackube/openstack/neutron-openvswitch-agent/ml2_conf.ini


docker run -d  --net host  \
    --name stackube_openstack_neutron_openvswitch_agent  \
    -v /etc/stackube/openstack/neutron-openvswitch-agent/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -v /run:/run:shared  \
    -v /lib/modules:/lib/modules:ro  \
    \
    -e "KOLLA_SERVICE_NAME=neutron-openvswitch-agent"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    --privileged  \
    kolla/centos-binary-neutron-openvswitch-agent:4.0.0  || exit 1

exit 0
