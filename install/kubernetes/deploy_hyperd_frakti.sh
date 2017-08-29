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
# - ``STREAMING_SERVER_ADDR``
# - ``FRAKTI_VERSION``  must be defined
#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x


## install libvirtd
yum install -y libvirt


## install hyperd
CENTOS7_QEMU_HYPER="http://hypercontainer-install.s3.amazonaws.com/qemu-hyper-2.4.1-3.el7.centos.x86_64.rpm"
CENTOS7_HYPERSTART="https://s3-us-west-1.amazonaws.com/hypercontainer-build/1.0-rc2/centos/hyperstart-0.8.1-1.el7.centos.x86_64.rpm"
CENTOS7_HYPER="https://s3-us-west-1.amazonaws.com/hypercontainer-build/1.0-rc2/centos/hyper-container-0.8.1-1.el7.centos.x86_64.rpm"

if rpm -qa | grep "hyper-container-0.8.1-1.el7.centos.x86_64" ; then
    true
else
    set -e
    yum install -y ${CENTOS7_QEMU_HYPER} ${CENTOS7_HYPERSTART} ${CENTOS7_HYPER}
    set +e
fi
set -e

cat > /etc/hyper/config << EOF
Kernel=/var/lib/hyper/kernel
Initrd=/var/lib/hyper/hyper-initrd.img
Hypervisor=qemu
StorageDriver=overlay
gRPCHost=127.0.0.1:22318

EOF


## install frakti
set +e
[ -f /usr/bin/frakti ] && rm -f /usr/bin/frakti
set -e
curl -sSL https://github.com/kubernetes/frakti/releases/download/${FRAKTI_VERSION}/frakti -o /usr/bin/frakti 
chmod +x /usr/bin/frakti

dockerInfo=`docker info `
cgroup_driver=`echo "${dockerInfo}" | awk '/Cgroup Driver/{print $3}' `
[ "${cgroup_driver}" ]

echo "[Unit]
Description=Hypervisor-based container runtime for Kubernetes
Documentation=https://github.com/kubernetes/frakti
After=network.target
[Service]
ExecStart=/usr/bin/frakti --v=3 \
          --log-dir=/var/log/frakti \
          --logtostderr=false \
          --cgroup-driver=${cgroup_driver} \
          --listen=/var/run/frakti.sock \
          --streaming-server-addr=${STREAMING_SERVER_ADDR} \
          --hyper-endpoint=127.0.0.1:22318
MountFlags=shared
#TasksMax=8192
LimitNOFILE=1048576
LimitNPROC=1048576
LimitCORE=infinity
TimeoutStartSec=0
Restart=on-abnormal
[Install]
WantedBy=multi-user.target
"  > /lib/systemd/system/frakti.service 


## start services
systemctl daemon-reload
systemctl enable hyperd frakti libvirtd
systemctl restart hyperd libvirtd
sleep 5
systemctl restart frakti
sleep 5

## check
hyperctl list 
pgrep -f '/usr/bin/frakti' 
[ -e /var/run/frakti.sock ] 



exit 0
