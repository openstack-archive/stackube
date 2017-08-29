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
# - ``RABBITMQ_PWD`` must be defined
#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o nounset
set -o pipefail
set -x

## rabbitmq 
mkdir -p /var/lib/stackube/openstack/rabbitmq  && \
docker run -d \
    --name stackube_openstack_rabbitmq \
    --net host  \
    -v /var/lib/stackube/openstack/rabbitmq:/var/lib/rabbitmq \
    --restart unless-stopped \
    rabbitmq:3.6 || exit 1

sleep 5
for i in 1 2 3 4 5; do
    docker exec stackube_openstack_rabbitmq rabbitmqctl status && break
    sleep $i
done
sleep 5

docker exec stackube_openstack_rabbitmq rabbitmqctl add_user openstack ${RABBITMQ_PWD} || exit 1
docker exec stackube_openstack_rabbitmq rabbitmqctl set_permissions openstack ".*" ".*" ".*" || exit 1

exit 0
