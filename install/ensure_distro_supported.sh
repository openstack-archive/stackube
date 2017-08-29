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

