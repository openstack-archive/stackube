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
# - ``OPENSTACK_ENDPOINT_IP``
# - ``MYSQL_HOST``, ``MYSQL_ROOT_PWD``
# - ``KEYSTONE_ADMIN_PWD``
# - ``KEYSTONE_CINDER_PWD``, ``MYSQL_CINDER_PWD``must be defined
#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x


## log dir
mkdir -p /var/log/stackube/openstack
chmod 777 /var/log/stackube/openstack


## register - Creating the Cinder service and endpoint
## v1
for IF in 'admin' 'internal' 'public'; do
    echo ${IF}
    docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost  -m kolla_keystone_service \
        -a "service_name=cinder
            service_type=volume
            description='Openstack Block Storage'
            endpoint_region=RegionOne
            url='https://${OPENSTACK_ENDPOINT_IP}:8777/v1/%(tenant_id)s'
            interface='${IF}'
            region_name=RegionOne
            auth='{{ openstack_keystone_auth }}'
            verify=False  " \
        -e "{'openstack_keystone_auth': {
               'auth_url': 'https://${OPENSTACK_ENDPOINT_IP}:35358/v3',
               'username': 'admin',
               'password': '${KEYSTONE_ADMIN_PWD}',
               'project_name': 'admin',
               'domain_name': 'default' } 
            }"
done

## v2
for VER in 'v2' ; do
    echo -e "\n--- ${VER} ---"
    for IF in 'admin' 'internal' 'public'; do
        echo ${IF}
        docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost  -m kolla_keystone_service \
            -a "service_name=cinder${VER}
                service_type=volume${VER}
                description='Openstack Block Storage'
                endpoint_region=RegionOne
                url='https://${OPENSTACK_ENDPOINT_IP}:8777/${VER}/%(tenant_id)s'
                interface='${IF}'
                region_name=RegionOne
                auth='{{ openstack_keystone_auth }}'
                verify=False  " \
            -e "{'openstack_keystone_auth': {
                   'auth_url': 'https://${OPENSTACK_ENDPOINT_IP}:35358/v3',
                   'username': 'admin',
                   'password': '${KEYSTONE_ADMIN_PWD}',
                   'project_name': 'admin',
                   'domain_name': 'default' } 
                }"
    done
done


## register -  Creating the Cinder project, user, and role
docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost  -m kolla_keystone_user \
    -a "project=service
        user=cinder
        password=${KEYSTONE_CINDER_PWD}
        role=admin
        region_name=RegionOne
        auth='{{ openstack_keystone_auth }}'
        verify=False  " \
    -e "{'openstack_keystone_auth': {
           'auth_url': 'https://${OPENSTACK_ENDPOINT_IP}:35358/v3',
           'username': 'admin',
           'password': '${KEYSTONE_ADMIN_PWD}',
           'project_name': 'admin',
           'domain_name': 'default' } 
        }"



# bootstrap - Creating Cinder database
docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost   -m mysql_db \
    -a "login_host=${MYSQL_HOST}
        login_port=3306
        login_user=root
        login_password=${MYSQL_ROOT_PWD}
        name=cinder"

# bootstrap - Creating Cinder database user and setting permissions
docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost   -m mysql_user \
    -a "login_host=${MYSQL_HOST}
        login_port=3306
        login_user=root
        login_password=${MYSQL_ROOT_PWD}
        name=cinder
        password=${MYSQL_CINDER_PWD}
        host=%
        priv='cinder.*:ALL'
        append_privs=yes"



# bootstrap_service - Running Cinder bootstrap container
docker run --net host  \
    --name stackube_openstack_bootstrap_cinder  \
    -v /etc/stackube/openstack/cinder-api/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -e "KOLLA_BOOTSTRAP="  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    kolla/centos-binary-cinder-api:4.0.0

sleep 2
docker rm stackube_openstack_bootstrap_cinder


## start_container - cinder-api
docker run -d  --net host  \
    --name stackube_openstack_cinder_api  \
    -v /etc/stackube/openstack/cinder-api/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    \
    -e "KOLLA_SERVICE_NAME=cinder-api"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    kolla/centos-binary-cinder-api:4.0.0

sleep 5


exit 0
