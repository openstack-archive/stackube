#!/bin/bash

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)


source ${programDir}/lib_common.sh || { echo "Error: 'source ${programDir}/lib_common.sh' failed!"; exit 1; }

MSG='Sorry, only CentOS 7.x supported for now.'

if ! is_fedora; then
    echo ${MSG}
    exit 1
fi

mainVersion=`echo ${os_RELEASE} | awk -F\. '{print $1}' `
if [ "${os_VENDOR}" == "CentOS" ] && [ "${mainVersion}" == "7" ]; then
    true
else
    echo ${MSG}
    exit 1
fi


exit 0

