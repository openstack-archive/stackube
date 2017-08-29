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

