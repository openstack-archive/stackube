#!/bin/bash
#
# Dependencies:
#
# - ``OPENSTACK_ENDPOINT_IP``
# - ``KEYSTONE_API_IP``
# - ``NEUTRON_API_IP``
# - ``CINDER_API_IP``  must be defined
#

programDir=`dirname $0`
programDir=$(readlink -f $programDir)
parentDir="$(dirname $programDir)"
programDirBaseName=$(basename $programDir)

set -o errexit
set -o nounset
set -o pipefail
set -x


## make certificates
HOST_IP=${OPENSTACK_ENDPOINT_IP}
SERVICE_HOST=${OPENSTACK_ENDPOINT_IP}
SERVICE_IP=${OPENSTACK_ENDPOINT_IP}
DATA_DIR='/etc/stackube/openstack/certificates'
source ${programDir}/../lib_tls.sh
mkdir -p ${DATA_DIR}
init_CA
init_cert


## log dir
mkdir -p /var/log/stackube/openstack
chmod 777 /var/log/stackube/openstack


## config files
mkdir -p /etc/stackube/openstack
cp -a ${programDir}/config_openstack/haproxy /etc/stackube/openstack/
sed -i "s/__OPENSTACK_ENDPOINT_IP__/${OPENSTACK_ENDPOINT_IP}/g" /etc/stackube/openstack/haproxy/haproxy.cfg
sed -i "s/__KEYSTONE_API_IP__/${KEYSTONE_API_IP}/g" /etc/stackube/openstack/haproxy/haproxy.cfg
sed -i "s/__NEUTRON_API_IP__/${NEUTRON_API_IP}/g" /etc/stackube/openstack/haproxy/haproxy.cfg
sed -i "s/__CINDER_API_IP__/${CINDER_API_IP}/g" /etc/stackube/openstack/haproxy/haproxy.cfg
# STACKUBE_CERT defined in lib_tls.sh
cat ${STACKUBE_CERT} > /etc/stackube/openstack/haproxy/haproxy.pem


## run
docker run -d  --net host  \
    --name stackube_openstack_haproxy  \
    -v /etc/stackube/openstack/haproxy/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    \
    -e "KOLLA_SERVICE_NAME=haproxy"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    --privileged  \
    kolla/centos-binary-haproxy:4.0.0


exit 0

