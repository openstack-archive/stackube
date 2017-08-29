#!/bin/bash
#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -x


export KUBECONFIG=/etc/kubernetes/admin.conf

for i in `seq 1 30`; do
    aaa=`kubectl get csr --all-namespaces | grep Pending | awk '{print $1}'`
    if [ "$aaa" ]; then
        for i in $aaa; do
            kubectl certificate approve $i || exit 1
        done
        sleep 5
    else
        break
    fi
done


exit 0
