#!/bin/bash
#
# Dependencies:
#
# - ``OPENSTACK_ENDPOINT_IP``
# - ``MYSQL_HOST``, ``MYSQL_ROOT_PWD``
# - ``KEYSTONE_ADMIN_PWD``
# - ``KEYSTONE_NEUTRON_PWD``, ``MYSQL_NEUTRON_PWD`` must be defined
#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x



## register - Creating the Neutron service and endpoint
for IF in 'admin' 'internal' 'public'; do 
    docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost  -m kolla_keystone_service \
        -a "service_name=neutron
            service_type=network
            description='Openstack Networking'
            endpoint_region=RegionOne
            url='https://${OPENSTACK_ENDPOINT_IP}:9697/'
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


## register - Creating the Neutron project, user, and role
docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost  -m kolla_keystone_user \
    -a "project=service
        user=neutron
        password=${KEYSTONE_NEUTRON_PWD}
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


# bootstrap - Creating Neutron database
docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost   -m mysql_db \
    -a "login_host=${MYSQL_HOST}
        login_port=3306
        login_user=root
        login_password=${MYSQL_ROOT_PWD}
        name=neutron"

# bootstrap - Creating Neutron database user and setting permissions
docker exec stackube_openstack_kolla_toolbox /usr/bin/ansible localhost   -m mysql_user \
    -a "login_host=${MYSQL_HOST}
        login_port=3306
        login_user=root
        login_password=${MYSQL_ROOT_PWD}
        name=neutron
        password=${MYSQL_NEUTRON_PWD}
        host=%
        priv='neutron.*:ALL'
        append_privs=yes"




## log dir
mkdir -p /var/log/stackube/openstack
chmod 777 /var/log/stackube/openstack


# bootstrap_service - Running Neutron bootstrap container
docker run --net host  \
    --name stackube_openstack_bootstrap_neutron  \
    -v /etc/stackube/openstack/neutron-server/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -e "KOLLA_BOOTSTRAP="  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    kolla/centos-binary-neutron-server:4.0.0

sleep 2
docker rm stackube_openstack_bootstrap_neutron


## start_container - neutron-server
docker run -d  --net host  \
    --name stackube_openstack_neutron_server  \
    -v /etc/stackube/openstack/neutron-server/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    \
    -e "KOLLA_SERVICE_NAME=neutron-server"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    kolla/centos-binary-neutron-server:4.0.0




exit 0
