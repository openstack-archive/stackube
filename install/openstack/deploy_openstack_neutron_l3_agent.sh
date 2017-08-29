#!/bin/bash
#
# Dependencies:
#
# - ``OVSDB_IP``
# - ``ML2_LOCAL_IP`` must be defined
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


## sysctl
sed -i '/^net\.ipv4\.ip_forward=/d' /etc/sysctl.conf
sed -i '/^net\.ipv4\.conf\.all\.rp_filter=/d' /etc/sysctl.conf
sed -i '/^net\.ipv4\.conf\.default\.rp_filter=/d' /etc/sysctl.conf

echo '
net.ipv4.ip_forward=1
net.ipv4.conf.all.rp_filter=0
net.ipv4.conf.default.rp_filter=0
' >> /etc/sysctl.conf

sysctl -p


## start_container - neutron-l3-agent
sed -i "s/__OVSDB_IP__/${OVSDB_IP}/g" /etc/stackube/openstack/neutron-l3-agent/ml2_conf.ini
sed -i "s/__LOCAL_IP__/${ML2_LOCAL_IP}/g" /etc/stackube/openstack/neutron-l3-agent/ml2_conf.ini

docker run -d  --net host  \
    --name stackube_openstack_neutron_l3_agent  \
    -v /etc/stackube/openstack/neutron-l3-agent/:/var/lib/kolla/config_files/:ro  \
    -v /var/log/stackube/openstack:/var/log/kolla/:rw  \
    -v /run:/run:shared  \
    \
    -e "KOLLA_SERVICE_NAME=neutron-l3-agent"  \
    -e "KOLLA_CONFIG_STRATEGY=COPY_ALWAYS" \
    \
    --restart unless-stopped \
    --privileged  \
    kolla/centos-binary-neutron-l3-agent:4.0.0


exit 0
