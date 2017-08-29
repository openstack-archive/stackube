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

set -o errexit
set -o nounset
set -o pipefail
set -x


source $(readlink -f $1)

[ "${CONTROL_NODE_PRIVATE_IP}" ]

[ "${NETWORK_NODES_PRIVATE_IP}" ]
#[ "${NETWORK_NODES_NEUTRON_EXT_IF}" ]

[ "${NEUTRON_PUBLIC_SUBNET}" ]

[ "${COMPUTE_NODES_PRIVATE_IP}" ]

[ "${STORAGE_NODES_PRIVATE_IP}" ]
[ "${STORAGE_NODES_CEPH_OSD_DATA_DIR}" ]


export OPENSTACK_ENDPOINT_IP="${CONTROL_NODE_PRIVATE_IP}"
export KEYSTONE_API_IP="${CONTROL_NODE_PRIVATE_IP}"
export NEUTRON_API_IP="${CONTROL_NODE_PRIVATE_IP}"
export CINDER_API_IP="${CONTROL_NODE_PRIVATE_IP}"

export MYSQL_HOST="${CONTROL_NODE_PRIVATE_IP}"
export MYSQL_ROOT_PWD=${MYSQL_ROOT_PWD:-MysqlRoot123}
export MYSQL_KEYSTONE_PWD=${MYSQL_KEYSTONE_PWD:-MysqlKeystone123}
export MYSQL_NEUTRON_PWD=${MYSQL_NEUTRON_PWD:-MysqlNeutron123}
export MYSQL_CINDER_PWD=${MYSQL_CINDER_PWD:-MysqlCinder123}

export RABBITMQ_HOST="${CONTROL_NODE_PRIVATE_IP}"
export RABBITMQ_PWD=${RABBITMQ_PWD:-rabbitmq123}

export KEYSTONE_ADMIN_PWD=${KEYSTONE_ADMIN_PWD:-KeystoneAdmin123}
export KEYSTONE_NEUTRON_PWD=${KEYSTONE_NEUTRON_PWD:-KeystoneNeutron123}
export KEYSTONE_CINDER_PWD=${KEYSTONE_CINDER_PWD:-KeystoneCinder123}




########## all nodes ##########

allIpList=`echo "
${CONTROL_NODE_PRIVATE_IP}
${NETWORK_NODES_PRIVATE_IP}
${COMPUTE_NODES_PRIVATE_IP}
${STORAGE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `

# kolla-toolbox
for IP in ${allIpList}; do
    ssh root@${IP} 'mkdir -p /etc/stackube/openstack /tmp/stackube_install'
    scp -r ${programDir}/openstack/config_openstack/kolla-toolbox root@${IP}:/etc/stackube/openstack/

    scp ${programDir}/openstack/deploy_openstack_kolla_toolbox.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "/bin/bash /tmp/stackube_install/deploy_openstack_kolla_toolbox.sh"
done



########## control node ##########

# db, mq, haproxy
/bin/bash ${programDir}/openstack/deploy_openstack_mariadb.sh
/bin/bash ${programDir}/openstack/deploy_openstack_rabbitmq.sh
/bin/bash ${programDir}/openstack/deploy_openstack_haproxy.sh

# keystone
/bin/bash ${programDir}/openstack/deploy_openstack_keystone.sh


# neutron server
function process_neutron_conf {
    local configFile="$1"
    sed -i "s/__RABBITMQ_HOST__/${RABBITMQ_HOST}/g" ${configFile}
    sed -i "s/__RABBITMQ_PWD__/${RABBITMQ_PWD}/g" ${configFile}
    sed -i "s/__NEUTRON_API_IP__/${NEUTRON_API_IP}/g" ${configFile}
    sed -i "s/__MYSQL_HOST__/${MYSQL_HOST}/g" ${configFile}
    sed -i "s/__OPENSTACK_ENDPOINT_IP__/${OPENSTACK_ENDPOINT_IP}/g" ${configFile}
    sed -i "s/__KEYSTONE_NEUTRON_PWD__/${KEYSTONE_NEUTRON_PWD}/g" ${configFile}
    sed -i "s/__MYSQL_NEUTRON_PWD__/${MYSQL_NEUTRON_PWD}/g" ${configFile}
}

mkdir -p /etc/stackube/openstack
cp -a ${programDir}/openstack/config_openstack/neutron-server /etc/stackube/openstack/
process_neutron_conf /etc/stackube/openstack/neutron-server/neutron.conf

source /etc/stackube/openstack/admin-openrc.sh 
cp -f ${OS_CACERT} /etc/stackube/openstack/neutron-server/haproxy-ca.crt

/bin/bash ${programDir}/openstack/deploy_openstack_neutron_server.sh


## cinder api
function process_cinder_conf {
    local cinderConfigFile="$1"
    sed -i "s/__CINDER_API_IP__/${CINDER_API_IP}/g" ${cinderConfigFile}
    sed -i "s/__RABBITMQ_HOST__/${RABBITMQ_HOST}/g" ${cinderConfigFile}
    sed -i "s/__RABBITMQ_PWD__/${RABBITMQ_PWD}/g" ${cinderConfigFile}
    sed -i "s/__MYSQL_CINDER_PWD__/${MYSQL_CINDER_PWD}/g" ${cinderConfigFile}
    sed -i "s/__MYSQL_HOST__/${MYSQL_HOST}/g" ${cinderConfigFile}
    sed -i "s/__OPENSTACK_ENDPOINT_IP__/${OPENSTACK_ENDPOINT_IP}/g" ${cinderConfigFile}
    sed -i "s/__KEYSTONE_CINDER_PWD__/${KEYSTONE_CINDER_PWD}/g" ${cinderConfigFile}
}
mkdir -p /etc/stackube/openstack
cp -a ${programDir}/openstack/config_openstack/cinder-api /etc/stackube/openstack/
process_cinder_conf /etc/stackube/openstack/cinder-api/cinder.conf

source /etc/stackube/openstack/admin-openrc.sh 
cp -f ${OS_CACERT} /etc/stackube/openstack/cinder-api/haproxy-ca.crt

/bin/bash ${programDir}/openstack/deploy_openstack_cinder_api.sh


# cinder scheduler
mkdir -p /etc/stackube/openstack
cp -a ${programDir}/openstack/config_openstack/cinder-scheduler /etc/stackube/openstack/
cp -f /etc/stackube/openstack/cinder-api/cinder.conf  /etc/stackube/openstack/cinder-scheduler/
/bin/bash ${programDir}/openstack/deploy_openstack_cinder_scheduler.sh


# cinder volume
docker exec stackube_ceph_mon ceph osd pool create cinder 128 128
docker exec stackube_ceph_mon ceph auth get-or-create client.cinder mon 'allow r' \
                 osd 'allow class-read object_prefix rbd_children, allow rwx pool=cinder'
docker exec stackube_ceph_mon /bin/bash -c 'ceph auth get-or-create client.cinder | tee /etc/ceph/ceph.client.cinder.keyring'

for IP in ${CONTROL_NODE_PRIVATE_IP} ; do 
    ssh root@${IP} 'mkdir -p /etc/stackube/openstack /tmp/stackube_install'
    scp -r ${programDir}/openstack/config_openstack/cinder-volume root@${IP}:/etc/stackube/openstack/
    scp -r /etc/stackube/openstack/cinder-api/cinder.conf \
           /var/lib/stackube/ceph/ceph_mon_config/{ceph.conf,ceph.client.cinder.keyring}  root@${IP}:/etc/stackube/openstack/cinder-volume/

    scp ${programDir}/openstack/deploy_openstack_cinder_volume.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "/bin/bash /tmp/stackube_install/deploy_openstack_cinder_volume.sh"
done




########## network nodes ##########

# neutron l3_agent
for IP in `echo ${NETWORK_NODES_PRIVATE_IP} | sed -e 's/,/ /g' ` ; do 
    ssh root@${IP} 'mkdir -p /etc/stackube/openstack /tmp/stackube_install'
    scp -r ${programDir}/openstack/config_openstack/neutron-l3-agent root@${IP}:/etc/stackube/openstack/
    scp -r /etc/stackube/openstack/neutron-server/neutron.conf \
           ${programDir}/openstack/config_openstack/neutron-server/ml2_conf.ini  root@${IP}:/etc/stackube/openstack/neutron-l3-agent/

    scp ${programDir}/openstack/deploy_openstack_neutron_l3_agent.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "export OVSDB_IP='${IP}'
                    export ML2_LOCAL_IP='${IP}'
                    /bin/bash /tmp/stackube_install/deploy_openstack_neutron_l3_agent.sh"
done


# neutron dhcp_agent
for IP in `echo ${NETWORK_NODES_PRIVATE_IP} | sed -e 's/,/ /g' ` ; do 
    ssh root@${IP} 'mkdir -p /etc/stackube/openstack /tmp/stackube_install'
    scp -r ${programDir}/openstack/config_openstack/neutron-dhcp-agent root@${IP}:/etc/stackube/openstack/
    scp -r /etc/stackube/openstack/neutron-server/neutron.conf \
           ${programDir}/openstack/config_openstack/neutron-server/ml2_conf.ini  root@${IP}:/etc/stackube/openstack/neutron-dhcp-agent/

    scp ${programDir}/openstack/deploy_openstack_neutron_dhcp_agent.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "export OVSDB_IP='${IP}'
                    export ML2_LOCAL_IP='${IP}'
                    /bin/bash /tmp/stackube_install/deploy_openstack_neutron_dhcp_agent.sh"
done


# neutron lbaas_agent
for IP in `echo ${NETWORK_NODES_PRIVATE_IP} | sed -e 's/,/ /g' ` ; do 
    ssh root@${IP} 'mkdir -p /etc/stackube/openstack /tmp/stackube_install'
    scp -r ${programDir}/openstack/config_openstack/neutron-lbaas-agent root@${IP}:/etc/stackube/openstack/
    scp -r /etc/stackube/openstack/neutron-server/neutron.conf \
           ${programDir}/openstack/config_openstack/neutron-server/{ml2_conf.ini,neutron_lbaas.conf}  root@${IP}:/etc/stackube/openstack/neutron-lbaas-agent/

    scp ${programDir}/openstack/deploy_openstack_neutron_lbaas_agent.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "export OVSDB_IP='${IP}'
                    export ML2_LOCAL_IP='${IP}'
                    export KEYSTONE_API_IP='${KEYSTONE_API_IP}'
                    export KEYSTONE_NEUTRON_PWD='${KEYSTONE_NEUTRON_PWD}'
                    /bin/bash /tmp/stackube_install/deploy_openstack_neutron_lbaas_agent.sh"
done





########## control & network & compute nodes ##########

# openvswitch agent (deploy on control node for k8s master)
allIpList=`echo "
${CONTROL_NODE_PRIVATE_IP}
${NETWORK_NODES_PRIVATE_IP}
${COMPUTE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `
for IP in ${allIpList}; do
    ssh root@${IP} 'mkdir -p /etc/stackube/openstack /tmp/stackube_install'
    scp -r ${programDir}/openstack/config_openstack/{openvswitch-db-server,openvswitch-vswitchd,neutron-openvswitch-agent} root@${IP}:/etc/stackube/openstack/
    scp -r /etc/stackube/openstack/neutron-server/neutron.conf ${programDir}/openstack/config_openstack/neutron-server/ml2_conf.ini  root@${IP}:/etc/stackube/openstack/neutron-openvswitch-agent/

    scp ${programDir}/openstack/deploy_openstack_neutron_openvswitch_agent.sh root@${IP}:/tmp/stackube_install/
    ssh root@${IP} "export OVSDB_IP='${IP}'
                    export ML2_LOCAL_IP='${IP}'
                    /bin/bash /tmp/stackube_install/deploy_openstack_neutron_openvswitch_agent.sh"
done

# network nodes: NEUTRON_EXT_IF
networkIpList=(`echo "${NETWORK_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g'`)
neutronExtIfList=(`echo "${NETWORK_NODES_NEUTRON_EXT_IF}" | sed -e 's/,/\n/g'`)
[ ${#networkIpList[@]} -eq ${#neutronExtIfList[@]} ]
MAX=$((${#networkIpList[@]} - 1))
for i in `seq 0 ${MAX}`; do
    IP="${networkIpList[$i]}"
    extIf="${neutronExtIfList[$i]}"
    echo -e "\n------ ${IP} ${extIf} ------"
    ssh root@${IP} "docker exec stackube_openstack_openvswitch_db /usr/local/bin/kolla_ensure_openvswitch_configured br-ex ${extIf}"
done





######### compute node ############

# certificate for kubestack
allIpList=`echo "
${COMPUTE_NODES_PRIVATE_IP}" | sed -e 's/,/\n/g' | sort | uniq `
for IP in ${allIpList}; do
    scp -r /etc/stackube/openstack/certificates/CA/int-ca/ca-chain.pem root@${IP}:/usr/share/pki/ca-trust-source/anchors/stackube-chain.pem
    ssh root@${IP} "update-ca-trust"
done





######### control node ############

# create public network and subnet
yum install centos-release-openstack-ocata.noarch -y
yum install python-openstackclient -y

source /etc/stackube/openstack/admin-openrc.sh
openstack network create --external --provider-physical-network physnet1 --provider-network-type flat public_1

# NEUTRON_PUBLIC_SUBNET='subnet-range;gateway;allocation-pool'
SUBNET=`echo "${NEUTRON_PUBLIC_SUBNET}" | awk -F\; '{print $1}'`
GATEWAY=`echo "${NEUTRON_PUBLIC_SUBNET}" | awk -F\; '{print $2}'`
POOL=`echo "${NEUTRON_PUBLIC_SUBNET}" | awk -F\; '{print $3}'`
openstack subnet create  public_1-subnet_1  \
    --subnet-range "${SUBNET}"  --gateway "${GATEWAY}"  --allocation-pool "${POOL}"  --no-dhcp  --network public_1


# check
openstack network list
openstack subnet list
openstack endpoint list
