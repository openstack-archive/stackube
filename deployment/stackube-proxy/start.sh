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

# Ensure all variables are defined.
set -u

TMP_CONF='/stackube.conf.tmp'
# Check environment variables before any real actions.
for i in 'AUTH_URL' 'USERNAME' 'PASSWORD' 'TENANT_NAME' 'DOMAIN_ID' 'REGION' 'EXT_NET_ID';do
	if [ "${!i}" ];then
		echo "environment variable $i = ${!i}"
	else
		echo "environment variable $i is empty, exit..."
		exit
	fi
done

# Insert parameters.
sed -i s~_AUTH_URL_~${AUTH_URL:-}~g $TMP_CONF
sed -i s/_USERNAME_/${USERNAME:-}/g $TMP_CONF
sed -i s/_PASSWORD_/${PASSWORD:-}/g $TMP_CONF
sed -i s/_TENANT_NAME_/${TENANT_NAME:-}/g $TMP_CONF
sed -i s/_DOMAIN_ID_/${DOMAIN_ID:-}/g $TMP_CONF
sed -i s/_REGION_/${REGION:-}/g $TMP_CONF
sed -i s/_EXT_NET_ID_/${EXT_NET_ID:-}/g $TMP_CONF

# Move the temporary stackube config into place.
STACKUBE_CONFIG_PATH='/etc/stackube.conf'
mv $TMP_CONF $STACKUBE_CONFIG_PATH
echo "Wrote stackube config: $(cat ${STACKUBE_CONFIG_PATH})"

# Start stackube-proxy in-cluster.
./stackube-proxy --kubeconfig="" --v=3