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


programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -x


## remove docker containers
stackubeCephConstaners=`docker ps -a | awk '{print $NF}' | grep '^stackube_ceph_' `
if [ "${stackubeCephConstaners}" ]; then
    docker rm -f $stackubeCephConstaners || exit 1
fi

## rm dirs
rm -fr /etc/stackube/ceph  /var/log/stackube/ceph  /var/lib/stackube/ceph  ${CEPH_OSD_DATA_DIR} || exit 1



exit 0

