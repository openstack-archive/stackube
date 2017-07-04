# devstack plugin

devstack plugin for stackube.

## All-in-one

```sh
# create stack user
sudo useradd -s /bin/bash -d /opt/stack -m stack
echo "stack ALL=(ALL) NOPASSWD: ALL" | sudo tee /etc/sudoers.d/stack
sudo su - stack

git clone https://git.openstack.org/openstack-dev/devstack -b stable/ocata
cd devstack
```

Create `local.conf` from [local.conf.sample](local.conf.sample) and then run `./stack.sh` to install.

```sh
./stack.sh
```

Wait a while for installation compelete, then setup kubernetes and OpenStack client:

```sh
# Kubernetes
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl cluster-info

# OpenStack
source openrc admin admin
openstack service list
```

## Add a node

```sh
# create stack user
sudo useradd -s /bin/bash -d /opt/stack -m stack
echo "stack ALL=(ALL) NOPASSWD: ALL" | sudo tee /etc/sudoers.d/stack
sudo su - stack

git clone https://git.openstack.org/openstack-dev/devstack -b stable/ocata
cd devstack
```

Create `local.conf` from [local.conf.node.sample](local.conf.node.sample), set `HOST_IP` to local host's IP, set `SERVICE_HOST` to master's IP and set `KUBEADM_TOKEN` to kubeadm token (could be got by `kubeadm token list`).

Then run `./stack.sh` to install.

