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


set -x

systemctl start docker &> /dev/null

sleep 2

docker info &> /dev/null

if [ "$?" != "0" ]; then 
    cat > /etc/yum.repos.d/docker.repo  << EOF
[docker-repo]
name=Docker main Repository
baseurl=https://yum.dockerproject.org/repo/main/centos/7
enabled=1
gpgcheck=1
gpgkey=https://yum.dockerproject.org/gpg
EOF
    yum install docker-engine-1.12.6 docker-engine-selinux-1.12.6 -y || exit 1
    #sed -i 's|ExecStart=.*|ExecStart=/usr/bin/dockerd  --storage-opt dm.mountopt=nodiscard --storage-opt dm.blkdiscard=false|g' /usr/lib/systemd/system/docker.service
    sed -i 's|ExecStart=.*|ExecStart=/usr/bin/dockerd  -s overlay |g' /usr/lib/systemd/system/docker.service
    systemctl daemon-reload  || exit 1
    systemctl enable docker || exit 1
    systemctl start  docker || exit 1
fi

sleep 5

docker info &> /dev/null || exit 1


exit 0

