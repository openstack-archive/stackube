#!/bin/bash

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)


setenforce 0
sed -i 's/SELINUX=enforcing/SELINUX=disabled/g' /etc/selinux/config


set -o errexit
set -o nounset
set -o pipefail
set -x


## install kubeadm kubectl kubelet
cat > /etc/yum.repos.d/kubernetes.repo << EOF
[kubernetes]
name=Kubernetes
baseurl=http://yum.kubernetes.io/repos/kubernetes-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg
       https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
EOF

yum install -y kubelet-1.7.4-0 kubeadm-1.7.4-0 kubectl-1.7.4-0

# configure_kubelet
unitFile='/etc/systemd/system/kubelet.service.d/10-kubeadm.conf'
sed -i '/^Environment="KUBELET_EXTRA_ARGS=/d'  ${unitFile} 
sed -i '/\[Service\]/aEnvironment="KUBELET_EXTRA_ARGS=--container-runtime=remote --container-runtime-endpoint=/var/run/frakti.sock --feature-gates=AllAlpha=true"'  ${unitFile} 


systemctl daemon-reload
systemctl enable kubelet



exit 0
