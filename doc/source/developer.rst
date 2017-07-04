Developer Document
=====================================

This page describes how to setup a working development environment that can be used in developing stackube on Ubuntu or CentOS. These instructions assume you're already installed git, golang and python on your host.

=================
Stackube controller
=================

--------
Build
--------

::

  make build

The binary will be placed at:

::

  _output/stackube-controller

--------
Start
--------

::

  # Please replace 10.123.0.x with your own external network
  # and remember the id of your created external network
  neutron net-create br-ex --router:external=True --shared
  neutron subnet-create --ip_version 4 --gateway 10.123.0.1 br-ex 10.123.0.0/16 --allocation-pool start=10.123.0.2,end=10.123.0.200 --name public-subnet

  # Create configure file for Stackube
  cat >/etc/stackube.conf <<EOF
  [Global]
  auth-url = https://<your-keystone-ip:port>/identity_admin/v2.0 
  username = admin
  password = password
  tenant-name = admin
  region = RegionOne
  ext-net-id = <id-of-your-external-network>
  EOF

  # Start stackube controller
  ./_output/stackube-controller --v=5



--------
Test
--------

1. Initialize Neutron

1. Prepare Tenant and Network

::

  $ cat test-tenant.yaml

  apiVersion: "stackube.kubernetes.io/v1"
  kind: Tenant
  metadata:
    name: test
  spec:
    username: "test"
    password: "password"


  $ cat test-network.yaml

  apiVersion: "stackube.kubernetes.io/v1"
  kind: Network
  metadata:
    name: test-net
    namespace: test
  spec:
    cidr: "192.168.0.0/24"
    gateway: "192.168.0.1"

2. Create new Tenant and its Network in Kubernetes

::

  $ export KUBECONFIG=/etc/kubernetes/admin.conf
  $ kubectl create -f ~/test-tenant.yaml
  $ kubectl create -f ~/test-network.yaml

3. Check the Network and Tenant is created in Neutron by Stackube controller

::

  $ source ~/keystonerc_admin
  $ neutron net-list
  +--------------------------------------+----------------------+----------------------------------+----------------------------------------------------------+
  | id                                   | name                 | tenant_id                        | subnets                                                  |
  +--------------------------------------+----------------------+----------------------------------+----------------------------------------------------------+
  | 421d913a-a269-408a-9765-2360e202ad5b | kube_test_test-net | 915b36add7e34018b7241ab63a193530 | bb446a53-de4d-4546-81fc-8736a9a88e3a 192.168.0.0/24      |

4. Check Network object is created in Kubernetes

::

 $ kubectl get network test-net --namespace=test
  NAME        KIND
  test-net   Network.v1.stackube.kubernetes.io

5. Delete the Network from Kubernetes

::

  $ kubectl delete -f test-network.yaml

6. Check Network in Neutron is also deleted by Stackube controller

::

  $ neutron net-list
  +--------------------------------------+---------+----------------------------------+----------------------------------------------------------+
  | id                                   | name    | tenant_id                        | subnets                                                  |
  +--------------------------------------+---------+----------------------------------+----------------------------------------------------------+
