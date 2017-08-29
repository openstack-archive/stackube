#!/bin/bash

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

