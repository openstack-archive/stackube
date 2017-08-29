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
# - ``OPENSTACK_ENDPOINT_IP``, ``KEYSTONE_API_IP``
# - ``MYSQL_HOST``, ``MYSQL_ROOT_PWD``
# - ``MYSQL_KEYSTONE_PWD``, ``KEYSTONE_ADMIN_PWD``  must be defined
#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x


## create db
docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost -m mysql_db  \
    -a "login_host=${MYSQL_HOST}
        login_port=3306
        login_user=root
        login_password=${MYSQL_ROOT_PWD}
        name=keystone"

docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost -m mysql_user  \
    -a "login_host=${MYSQL_HOST}
        login_port=3306
        login_user=root
        login_password=${MYSQL_ROOT_PWD}
        name=keystone
        password=${MYSQL_KEYSTONE_PWD}
        host=%
        priv=keystone.*:ALL
        append_privs=yes "


## log dir
mkdir -p /var/log/stackube/openstack
chmod 777 /var/log/stackube/openstack


## config files
mkdir -p /etc/stackube/openstack
cp -a ${programDir}/config_openstack/keystone /etc/stackube/openstack/
sed -i "s/__MYSQL_HOST__/${MYSQL_HOST}/g" /etc/stackube/openstack/keystone/keystone.conf
sed -i "s/__MYSQL_KWYSTONE_PWD__/${MYSQL_KEYSTONE_PWD}/g" /etc/stackube/openstack/keystone/keystone.conf
sed -i "s/__KEYSTONE_API_IP__/${KEYSTONE_API_IP}/g" /etc/stackube/openstack/keystone/wsgi-keystone.conf


# bootstrap_service
docker run --net host  \
    --name stackube_openstack_bootstrap_keystone  \
    -v /etc/stackube/openstack/keystone/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -e "KOLLA_BOOTSTRAP="  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    kolla/centos-binary-keystone:4.0.0

docker rm stackube_openstack_bootstrap_keystone

docker run -d  --net host  \
    --name stackube_openstack_keystone  \
    -v /etc/stackube/openstack/keystone/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -e "KOLLA_SERVICE_NAME=keystone"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    --restart unless-stopped \
    kolla/centos-binary-keystone:4.0.0

sleep 10

# register
docker exec stackube_openstack_keystone kolla_keystone_bootstrap admin ${KEYSTONE_ADMIN_PWD} admin admin \
    https://${OPENSTACK_ENDPOINT_IP}:35358/v3 \
    https://${OPENSTACK_ENDPOINT_IP}:5001/v3 \
    https://${OPENSTACK_ENDPOINT_IP}:5001/v3 \
    RegionOne

docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost -m os_keystone_role  -a "name=_member_  auth='{{ openstack_keystone_auth }}' verify=False"  \
    -e "{'openstack_keystone_auth': {
           'auth_url': 'https://${OPENSTACK_ENDPOINT_IP}:35358/v3',
           'username': 'admin',
           'password': '${KEYSTONE_ADMIN_PWD}',
           'project_name': 'admin',
           'domain_name': 'default' } 
        }"


cat > /etc/stackube/openstack/admin-openrc.sh << EOF
export OS_PROJECT_DOMAIN_NAME=default
export OS_USER_DOMAIN_NAME=default
export OS_PROJECT_NAME=admin
export OS_TENANT_NAME=admin
export OS_USERNAME=admin
export OS_PASSWORD=${KEYSTONE_ADMIN_PWD}
export OS_AUTH_URL=https://${OPENSTACK_ENDPOINT_IP}:35358/v3
export OS_INTERFACE=internal
export OS_IDENTITY_API_VERSION=3
export OS_CACERT=/etc/stackube/openstack/certificates/CA/int-ca/ca-chain.pem
EOF

exit 0

