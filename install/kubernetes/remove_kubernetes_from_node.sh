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

if command -v kubeadm > /dev/null 2>&1; then
    kubeadm reset  || exit 1
fi


systemctl stop hyperd kubelet
yum remove -y  kubelet  kubeadm  kubectl  qemu-hyper  hyperstart  hyper-container  || exit 1
rm -fr  /etc/kubernetes  /var/lib/kubelet  /var/run/kubernetes

systemctl stop frakti
rm -f  /usr/bin/frakti  /lib/systemd/system/frakti.service  || exit 1
systemctl daemon-reload



exit 0

