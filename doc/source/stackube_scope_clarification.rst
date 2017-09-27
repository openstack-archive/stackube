==============
Stackube Scope
==============

A multi-tenant and secure Kubernetes deployment enabled by OpenStack
core components.

Not another “Kubernetes on OpenStack” project
=============================================

Stackube is a standard upstream Kubernetes deployment with:

#. Mixed container runtime of Docker (Linux container) and HyperContainer (hypervisor-based container)

#. Keystone for tenant management

#. Neutron for container network

#. Cinder for persistent volume

The main difference between Stackube with existing container service
project in OpenStack foundation (e.g. Magnum) is: **Stackube works
alongside OpenStack, not on OpenStack**. 

This means:

#. Only standalone vanilla OpenStack components are required

#. Traditional VMs are not required because HyperContainer will provide hypervisor level isolation for containerized workloads.

#. All the components mentioned above are managed by Kubernetes plugin API.

What‘s inside Stackube repo?
============================

#. Keystone RBAC plugin

#. Neutron CNI plugin

   * With a k8s Network object controller

#. Standard k8s upstream Cinder plugin with block device mode

#. Deployment scripts and guide

#. Other documentations

Please note:

#. Plugins above will be deployed as system Pod and DaemonSet.

#. All other Kubernetes volumes are also supported in Stackube, while k8s Cinder plugin with block device mode can provide better performance in mixed runtime which will be preferred by default.

What’s the difference between other plugin projects?
====================================================

#. Kuryr

   * This is a Neutron network plugin for Docker network model, which is not directly supported in Kubernetes. Kuryr can provide CNI interface, but Stackube also requires tenant aware network management which is not included in Kuryr. We will evaluate and propose our multi-tenant model to kuryr-kubernetes as a long term effort, then we can move to use kuryr-kubernetes as the default network plugin.

#. Fuxi

   * This is a Cinder volume plugin for Docker volume model, which is not supported in latest CRI based Kubernetes (using k8s volume plugin for now, and soon CSI). Also, Stackube prefers a “block-device to Pod” mode in volume plugin when HyperContainer runtime is enabled, which is not supported in Fuxi.

#. K8s-cloud-provider

   * This is a “Kubernetes on OpenStack” integration which requires full functioning OpenStack deployment.

#. Zun

   * This is a OpenStack API container service, while Stackube exposes well-known Kubernetes API and does not require full OpenStack deployment.

As summary, one distinguishable difference is that plugins in Stackube
are designed to enable hard multi-tenancy in Kubernetes as a whole
solution, while the other OpenStack plugin projects do not address this
and solely focus on just integrating with Kubernetes/Docker as-is. There
are many gaps to fill when use them to build a real multi-tenant cloud,
for example, how tenants cooperate with networks in k8s.

Another difference is Stackube use mixed container runtimes mode of k8s
to enable secure runtime, which is not in scope of existing foundation
projects. In fact, all plugins in Stackube should work well for both
Docker and HyperContainer.

The architecture of Stackube is fully decoupled and it would be easy for
us (and we’d like to) integrate it with any OpenStack-Kubernetes plugin.
But right now, we hope to keep everything as simple as possible and
focus on the core components.

Deployment workflow
=========================================

-----------------
On control nodes
-----------------

Install standalone Keystone, Neutron, Cinder (ceph rbd).
This can be done by any existing tools like devstack, RDO etc.

----------------
On other nodes
----------------

1. Install neutron L2 agents

   This can be done by any existing tools like devstack, RDO etc.

2. Install Kubernetes

   * Including container runtimes, CRI shims, CNI etc
   * This can be done by any existing tools like kubeadm etc

3. Deploy Stackube

::

  kubectl create -f stackube-configmap.yaml
  kubectl create -f deployment/stackube-proxy.yaml
  kubectl create -f deployment/stackube.yaml


This will deploy all Stackube plugins as Pods and DaemonSets to the
cluster. You can also deploy all these components in a single node.

After that, users can use Kubernetes API to manage containers with
hypervisor isolation, Neutron network, Cinder volume and tenant
awareness.
