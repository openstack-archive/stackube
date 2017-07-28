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

# Local version to install bindep packages
# Suitable for use for development

function is_fedora {
    [ -f /usr/bin/yum ] && cat /etc/*release | grep -q -e "Fedora"
}

PACKAGES=""
if ! which virtualenv; then
    PACKAGES="$PACKAGES virtualenv"
fi
if ! which make; then
    PACKAGES="$PACKAGES make"
fi
if [[ -n $PACKAGES ]]; then
    sudo apt-get -q --assume-yes install virtualenv
fi

# Check for bindep
if ! which bindep; then
    make bindep
fi

PACKAGES=$(make bindep || true)

# inspired from project-config install-distro-packages.sh
if apt-get -v >/dev/null 2>&1 ; then
    sudo apt-get -qq update
    sudo PATH=/usr/sbin:/sbin:$PATH DEBIAN_FRONTEND=noninteractive \
        apt-get -q --option "Dpkg::Options::=--force-confold" \
        --assume-yes install $PACKAGES
elif emerge --version >/dev/null 2>&1 ; then
    sudo emerge -uDNq --jobs=4 @world
    sudo PATH=/usr/sbin:/sbin:$PATH emerge -q --jobs=4 $PACKAGES
else
    is_fedora && YUM=dnf || YUM=yum
    sudo PATH=/usr/sbin:/sbin:$PATH $YUM install -y $PACKAGES
fi
