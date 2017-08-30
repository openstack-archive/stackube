Developer Documentation
=====================================

This page describes how to setup a working development environment that can be used in developing stackube on Ubuntu or
CentOS. These instructions assume you're already installed git, golang and python on your host.

=========
Design Tips
=========

The Stackube project is very simple. The main part of it is a stackube-controller, which use Kubernetes Customized Resource Definition (CRD, previous TPR) to:

1. Manage tenants based on namespace change in k8s
2. Manage RBAC based on namespace change in k8s
3. Manage networks based on tenants change in k8s

The tenant is a CRD which maps to Keystone tenant, the network is a CRD which maps to Neutron network. We also have a kubestack binary which is the CNI plug-in for Neutron.

Also, Stackube has it's own stackube-proxy to replace kube-proxy because network in Stackube is L2 isolated, so we need a multi-tenant version kube-proxy here.

We also replaced kube-dns in k8s for the same reason: we need to have a kube-dns running in every namespace instead of a global DNS server because namespaces are isolated.

You can see that:  Stackube cluster = upstream Kubernetes + several our own add-ons + standalone OpenStack components.

Please note: Cinder RBD based block device as volume is implemented in https://github.com/kubernetes/frakti, you need to contribute there if you have any idea and build a new stackube/flex-volume Docker image for Stackube to use.

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

  stackube/stackube-proxy:v1.0beta
  stackube/stackube-controller:v1.0beta
  stackube/kubestack:v1.0beta

===========================
(Optional) Configure Stackube
===========================

If you deployed Stackube by following official guide, you can skip this part.

But if not, these steps below are needed to make sure your Stackube cluster work.

Please note the following parts suppose you have already deployed an environment of OpenStack and Kubernetes on same baremetal host. And don't forget to setup `--experimental-keystone-url` for kube-apiserver, e.g.

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


And create configure file for Stackube.

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
    keyring: "AQBZU5lZ/Z7lEBAAJuC17RYjjqIUANs2QVn7pw=="
  EOF

Then deploy stackube components:

::

  kubectl create -f stackube-configmap.yaml
  kubectl create -f deployment/stackube-proxy.yaml
  kubectl create -f deployment/stackube.yaml
  kubectl create -f deployment/flexvolume/flexvolume-ds.yaml


Now, you are ready to try Stackube features.
