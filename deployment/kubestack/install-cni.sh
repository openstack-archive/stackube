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

KUBESTACK_CONFIG_PATH='/host/etc/kubestack.conf'
# If specified, overwrite the kubestack configuration file.
if [ "${KUBESTACK_CONFIG:-}" != "" ]; then
cat >$KUBESTACK_CONFIG_PATH <<EOF
${KUBESTACK_CONFIG:-}
EOF
echo "Wrote CNI config: $(cat ${KUBESTACK_CONFIG_PATH})"
fi

while true; do 
	sleep 3600; 
done
