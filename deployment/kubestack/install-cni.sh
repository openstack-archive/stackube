#!/bin/sh

# Script to install kubestack CNI on a Kubernetes host.
# - Expects the host CNI binary path to be mounted at /host/opt/cni/bin.
# - Expects the host CNI network config path to be mounted at /host/etc/cni/net.d.
# - Expects the desired CNI config in the /host/etc/cni/net.d/10-kubestack.conf.
# - Expects the desired kubestack config in the KUBESTACK_CONFIG env variable.

# Ensure all variables are defined.
set -u

# Clean up any existing binaries / config / assets.
rm -f /host/opt/cni/bin/kubestack
rm -f /host/etc/cni/net.d/10-kubestack.conf
rm -f /etc/kubestack.conf

# Place the new binaries if the directory is writeable.
if [ -w "/host/opt/cni/bin/" ]; then
	cp /opt/cni/bin/kubestack /host/opt/cni/bin/
	echo "Wrote kubestack CNI binaries to /host/opt/cni/bin/"
	echo "CNI plugin version: $(/host/opt/cni/bin/kubestack -v)"
fi

# Place the new CNI network config if the directory is writeable.
if [ -w "/host/etc/cni/net.d/" ]; then
	cp /etc/cni/net.d/10-kubestack.conf /host/etc/cni/net.d/
	echo "Wrote CNI network config to /host/etc/cni/net.d/"
	echo "CNI config: $(cat /host/etc/cni/net.d/10-kubestack.conf)"
fi

TMP_CONF='/kubestack.conf.tmp'
# Check environment variables before any real actions.
if [ -z $AUTH_URL ];then
    echo "environment variable AUTH_URL has not been setted or is empty"
    exit
else
    echo "AUTH_URL = $AUTH_URL"
fi

if [ -z $USERNAME ];then
    echo "environment variable USERNAME has not been setted or is empty"
    exit
else
    echo "USERNAME = $USERNAME"
fi

if [ -z $PASSWORD ];then
    echo "environment variable PASSWORD has not been setted or is empty"
    exit
else
    echo "PASSWORD = $PASSWORD"
fi

if [ -z $TENANT_NAME ];then
    echo "environment variable TENANT_NAME has not been setted or is empty"
    exit
else
    echo "TENANT_NAME = $TENANT_NAME"
fi

if [ -z $REGION ];then
    echo "environment variable REGION has not been setted or is empty"
    exit
else
    echo "REGION = $REGION"
fi

if [ -z $EXT_NET_ID ];then
    echo "environment variable EXT_NET_ID has not been setted or is empty"
    exit
else
    echo "EXT_NET_ID = $EXT_NET_ID"
fi

if [ -z $PLUGIN_NAME ];then
    echo "environment variable PLUGIN_NAME has not been setted or is empty"
    exit
else
    echo "PLUGIN_NAME = $PLUGIN_NAME"
fi

if [ -z $INTEGRATION_BRIDGE ];then
    echo "environment variable INTEGRATION_BRIDGE has not been setted or is empty"
    exit
else
    echo "INTEGRATION_BRIDGE = $INTEGRATION_BRIDGE"
fi

# Insert parameters.
sed -i s~_AUTH_URL_~${AUTH_URL:-}~g $TMP_CONF
sed -i s/_USERNAME_/${USERNAME:-}/g $TMP_CONF
sed -i s/_PASSWORD_/${PASSWORD:-}/g $TMP_CONF
sed -i s/_TENANT_NAME_/${TENANT_NAME:-}/g $TMP_CONF
sed -i s/_REGION_/${REGION:-}/g $TMP_CONF
sed -i s/_EXT_NET_ID_/${EXT_NET_ID:-}/g $TMP_CONF
sed -i s/_PLUGIN_NAME_/${PLUGIN_NAME:-}/g $TMP_CONF
sed -i s/_INTEGRATION_BRIDGE_/${INTEGRATION_BRIDGE:-}/g $TMP_CONF

# Move the temporary kubestack config into place.
KUBESTACK_CONFIG_PATH='/host/etc/kubestack.conf'
mv $TMP_CONF $KUBESTACK_CONFIG_PATH
echo "Wrote kubestack config: $(cat ${KUBESTACK_CONFIG_PATH})"

while true; do
	sleep 3600;
done
