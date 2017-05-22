==============
Stackube Scope
==============

A multi-tenant and secure Kubernetes deployment enabled by OpenStack
core components.

Not another “Kubernetes on OpenStack” project

Stackube is a standard upstream Kubernetes deployment with:

1. Mixed container runtime of Docker (Linux container) and
       HyperContainer (hypervisor-based container)

2. Keystone for tenant management

3. Neutron for container network

4. Cinder for persistent volume

The main difference between Stackube with existing container service
project in OpenStack foundation (e.g. Magnum) is: **Stackube works
alongside OpenStack, not on OpenStack**. This means:

1. Only standalone vanilla OpenStack components are required

2. Traditional VMs are not required because HyperContainer will provide
       hypervisor level isolation for containerized workloads.

3. All the components mentioned above are managed by Kubernetes plugin
       API.

What‘s inside Stackube repo?

1. Keystone RBAC plugin

2. Neutron CNI plugin

   a. With a k8s Network object controller

3. Standard k8s upstream Cinder plugin with block device mode

4. Deployment scripts and guide

5. Other documentations

Please note:

1. Plugins above will be deployed as system Pod and DaemonSet.

2. All other Kubernetes volume are also supported in Stackube, while k8s
       Cinder plugin with block device mode can provide better
       performance in mixed runtime which will be preferred by default.

What’s the difference between other plugin projects?

1. Kuryr

   a. This is a Neutron network plugin for Docker network model, which
          is not directly supported in Kubernetes. Kuryr can provide CNI
          interface, but Stackube also requires tenant aware network
          management which is not included in Kuryr.

2. Fuxi

   a. This is a Cinder volume plugin for Docker volume model, which is
          not supported in latest CRI based Kubernetes (using k8s volume
          plugin for now, and soon CSI). Also, Stackube prefers a
          “block-device to Pod” mode in volume plugin when
          HyperContainer runtime is enabled, which is not supported in
          Fuxi.

3. K8s-cloud-provider

   a. This is a “Kubernetes on OpenStack” integration which requires
          full functioning OpenStack deployment.

4. Zun

   a. This is a OpenStack API container service, while Stackube exposes
          well-known Kubernetes API and does not require no full
          OpenStack deployment.

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

A typical deployment workflow of Stackube

On control nodes:

1. Install standalone Keystone, Neutron, Cinder (ceph rbd)

   a. This can be done by any existing tool like devstack, RDO etc

On other nodes:

1. Install Kubernetes

   a. Including container runtimes, CRI shims, CNI etc

   b. This can be done by any existing tool like kubeadm etc

Deploy Stackube:

1. *kubectl apply -f stackube.yaml*

   a. This will deploy all Stackube plugins as Pods and DaemonSets to
          the cluster

(You can also deploy all these components in a single node)

After that, users can use Kubernetes API to manage containers with
hypervisor isolation, Neutron network, Cinder volume and tenant
awareness.
