# Stackube

Stackube is a Kubernetes-centric OpenStack distro. It uses Kubernetes, instead of Nova, as the compute
fabric controller, to provision containers as the compute instance, along with other OpenStack
services (e.g. Cinder, Neutron). It supports multiple container runtime technologies, e.g. Docker,
Hyper, and offers built-in soft / hard multi-tenancy (depending on the container runtime used).

Stackube uses the Apache v2.0 license. All library dependencies allow for
unrestricted distribution and deployment.

* Source: <https://git.openstack.org/cgit/openstack/stackube>
* Bugs: <https://bugs.launchpad.net/stackube>
* Blueprints: <https://blueprints.launchpad.net/stackube>
