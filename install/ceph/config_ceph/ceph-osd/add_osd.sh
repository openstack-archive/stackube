#!/bin/bash
#

set -x

cp --remove-destination /var/lib/kolla/config_files/{ceph.client.admin.keyring,ceph.conf} /etc/ceph/ || exit 1

ceph osd crush add-bucket __PUBLIC_IP__ host || exit 1
ceph osd crush move __PUBLIC_IP__ root=default || exit 1

num=`ceph osd create` || exit 1
echo $num || exit 1
mkdir -p /var/lib/ceph/osd/ceph-${num} || exit 1
ceph-osd -i ${num} --mkfs --mkkey || exit 1
ceph auth add osd.${num} osd 'allow *' mon 'allow profile osd' -i /var/lib/ceph/osd/ceph-${num}/keyring || exit 1
ceph osd crush add osd.${num} 1.0 host=__PUBLIC_IP__ || exit 1

chown ceph:ceph /var/lib/ceph/osd -R  || exit 1

ceph osd crush tree

exit 0

