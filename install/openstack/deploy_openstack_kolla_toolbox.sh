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

