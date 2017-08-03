Deployment Documentation
=====================================

This page describes how to setup a working development environment that can be used in developing stackube on Ubuntu or
CentOS. These instructions assume you're already installed git, golang and python on your host.

=========
Build
=========

Build binary:

::

  make

The binary will be placed at:

::

  _output/kubestack
  _output/stackube-controller
  _output/stackube-proxy

Build docker images:

::

  make docker

Three docker images will be built:

::

  stackube/stackube-proxy:v0.1
  stackube/stackube-controller:v0.1
  stackube/kubestack:v0.1

=========
Start
=========

The following parts suppose you have already deployed an environment of OpenStack and Kubernetes on same baremetal host.
If the cluster is not deployed via devstack, don't forget to setup `--experimental-keystone-url` for kube-apiserver, e.g.

::

    kube-apiserver --experimental-keystone-url=https://192.168.128.66:5000/v2.0 ...

Remove kube-dns deployment and kube-proxy daemonset if you have already running them.

::

  kubectl -n kube-system delete deployment kube-dns
  kubectl -n kube-system delete daemonset kube-proxy

If you have also configured a CNI network plugin, you should also remove it togather with CNI network config.

::

  # Remove CNI network components, e.g. deployments or daemonsets first.
  # Then remove CNI network config.
  rm -f /etc/cni/net.d/*

Then create external network in Neutron if there is no one.

::

  # Create an external network if there is no one.
  # Please replace 10.123.0.x with your own external network
  # and remember the id of your created external network
  neutron net-create br-ex --router:external=True --shared
  neutron subnet-create --ip_version 4 --gateway 10.123.0.1 br-ex 10.123.0.0/16 --allocation-pool start=10.123.0.2,end=10.123.0.200 --name public-subnet


Now, we are ready to deploy stackube components.

First create configure file for Stackube.

::

  # Remember to replace them with your own ones.
  cat >stackube-configmap.yaml <<EOF
  kind: ConfigMap
  apiVersion: v1
  metadata:
    name: stackube-config
    namespace: kube-system
  data:
    auth-url: "https://192.168.128.66/identity_admin/v2.0"
    username: "admin"
    password: "admin"
    tenant-name: "admin"
    region: "RegionOne"
    ext-net-id: "550370a3-4fc2-4494-919d-cae33f5b3de8"
    plugin-name: "ovs"
    integration-bridge: "br-int"
    user-cidr: "10.244.0.0/16"
    user-gateway: "10.244.0.1"
    kubernetes-host: "192.168.0.33"
    kubernetes-port: "6443"
  EOF

Then deploy stackube components:

::

  kubectl create -f stackube-configmap.yaml
  kubectl create -f deployment/stackube-proxy.yaml
  kubectl create -f deployment/stackube.yaml


=========
Test
=========

1. Create a new tenant

::

  $ cat test-tenant.yaml

  apiVersion: "stackube.kubernetes.io/v1"
  kind: Tenant
  metadata:
    name: test
  spec:
    username: "test"
    password: "password"

  $ kubectl create -f test-tenant.yaml

2. Check the auto-created namespace and network. Wait a while, the namespace and network for this tenant should be created automatically:

::

  $ kubectl get namespace test
  NAME      STATUS    AGE
  test     Active    58m

  $ kubectl -n test get network test -o yaml
  apiVersion: stackube.kubernetes.io/v1
  kind: Network
  metadata:
    clusterName: ""
    creationTimestamp: 2017-08-03T11:58:31Z
    generation: 0
    name: test
    namespace: test
    resourceVersion: "3992023"
    selfLink: /apis/stackube.kubernetes.io/v1/namespaces/test/networks/test
    uid: 11d452eb-7843-11e7-8319-68b599b7918c
  spec:
    cidr: 10.244.0.0/16
    gateway: 10.244.0.1
    networkID: ""
  status:
    state: Active

3. Check the Network and Tenant created in Neutron by Stackube controller.

::

  $ source ~/keystonerc_admin
  $ neutron net-list
  +--------------------------------------+----------------------+----------------------------------+----------------------------------------------------------+
  | id                                   | name                 | tenant_id                        | subnets                                                  |
  +--------------------------------------+----------------------+----------------------------------+----------------------------------------------------------+
  | 421d913a-a269-408a-9765-2360e202ad5b | kube-test-test       | 915b36add7e34018b7241ab63a193530 | bb446a53-de4d-4546-81fc-8736a9a88e3a 10.244.0.0/16       |

4. Check the kube-dns pods created in the new namespace.

::

  # kubectl -n test get pods
  NAME                        READY     STATUS    RESTARTS   AGE
  kube-dns-1476438210-37jv7   3/3       Running   0          1h

5. Create pods and services in the new namespace.

::

  # kubectl -n test run nginx --image=nginx
  deployment "nginx" created
  # kubectl -n test expose deployment nginx --port=80
  service "nginx" exposed

  # kubectl -n test get pods -o wide
  NAME                        READY     STATUS    RESTARTS   AGE       IP            NODE
  kube-dns-1476438210-37jv7   3/3       Running   0          1h        10.244.0.4    stackube
  nginx-4217019353-6gjxq      1/1       Running   0          27s       10.244.0.10   stackube

  # kubectl -n test run -i -t busybox --image=busybox sh
  If you don't see a command prompt, try pressing enter.
  / # nslookup nginx
  Server:    10.96.0.10
  Address 1: 10.96.0.10

  Name:      nginx
  Address 1: 10.108.57.129 nginx.test.svc.cluster.local

  / # wget -O- nginx
  Connecting to nginx (10.108.57.129:80)
  <!DOCTYPE html>
  <html>
  <head>
  <title>Welcome to nginx!</title>
  <style>
      body {
          width: 35em;
          margin: 0 auto;
          font-family: Tahoma, Verdana, Arial, sans-serif;
      }
  </style>
  </head>
  <body>
  <h1>Welcome to nginx!</h1>
  <p>If you see this page, the nginx web server is successfully installed and
  working. Further configuration is required.</p>

  <p>For online documentation and support please refer to
  <a href="http://nginx.org/">nginx.org</a>.<br/>
  Commercial support is available at
  <a href="http://nginx.com/">nginx.com</a>.</p>

  <p><em>Thank you for using nginx.</em></p>
  </body>
  </html>
  -                    100% |*********************************************************************|   612   0:00:00 ETA
  / #

6. Finally, remove the tenant.

::

  $ kubectl delete tenant test
  tenant "test" deleted

7. Check Network in Neutron is also deleted by Stackube controller

::

  $ neutron net-list
  +--------------------------------------+---------+----------------------------------+----------------------------------------------------------+
  | id                                   | name    | tenant_id                        | subnets                                                  |
  +--------------------------------------+---------+----------------------------------+----------------------------------------------------------+
