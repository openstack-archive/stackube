Stackube Release Note
=====================================

=========
1.0 Beta
=========

=========
Prelude
=========

This is the first release of Stackube: a secure, multi-tenant and Kubernetes centric OpenStack distribution.

=========
New Features
=========

1. Implemented a auth controller watches on tenant/networks/namespaces change and setups RBAC roles for tenant. This is how we match Kubernetes authorization control with OpenStack tenant by: tenant namespace 1:1 mapping. Controller is implemented by using CRD of Kubernetes.

2. Implemented a network controller watches on networks/namespaces change and operates OpenStack network based on network namespace 1:1 mapping. This is how to define Kubernetes multi-tenant network by using Neutron. Controller is implemented by using CRD of Kubernetes.

3. Implemented a CNI plug-in which name is kubestack. This is a CNI plug-in for Neutron and work with the network controller model mentioned above. When network of Neutron is ready, kubestack will be responsible to configure Pods to use this network by following standard CNI workflow.

4. Implemented stackube-proxy to replace default kube-proxy in Kubernetes so that proxy will be aware of multi-tenant network.

5. Implemented stackube DNS add-on to replace default kube-dns in Kubernetes so that DNS server will be aware of multi-tenant network.

6. Integrated cinder-flexvolume of kubernetes/frakti project so that hypervisor based container runtime (runV) in Stackube can mount Cinder volume (RBD) directly to VM-based Pod. This will bring better performance to user.

7. Improved stackube-proxy so that Neutron LBaaS based service can be used in Stackube when LoadBalancer type service is created in Kubernetes.

8. Created Docker images for all add-on and plug-in above so they can be deployed in Stackube by one command.

9. Add deployment documentation for Stackube which will install Kubernetes + mixed container runtime + CNI + volume plugin + standalone OpenStack components.


=========
Known Issues
=========

None

=========
Upgrade Notes
=========

None

=========
Deprecation Notes
=========

None

=========
Bug Fixes
=========

1. Fixed CNI network namespace is not cleaned properly

=========
Other Notes
=========

This is the first release of Stackube, welcome to fire bugs in https://bugs.launchpad.net/stackube during you exploration.
