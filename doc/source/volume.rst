Persistent volume in Stackube
=====================================

This page describes the persistent volume design and usage in Stackube.

=================
Use standard Kubernetes volume
=================

Stackube is a standard upstream Kubernetes cluster, so any type of `Kubernetes volumes 
<https://kubernetes.io/docs/concepts/storage/volumes/>`_. can be used here, for example:
::

  apiVersion: v1
  kind: PersistentVolume
  metadata:
    name: nfs
  spec:
    capacity:
      storage: 1Mi
    accessModes:
      - ReadWriteMany
    nfs:
      # FIXME: use the right IP
      server: 10.244.1.4
      path: "/exports"

Please note since Stackube is a baremetal k8s cluster, cloud provider based volume like GCE, AWS etc is not supported by default.

But unless you are using emptyDir or hostPath, we highly recommend the "Cinder RBD based block device as volume" described below in Stackube, because this volume type will bring you much higher performance.

==================================
Use Cinder RBD based block device as volume
==================================

The reason this volume type is recommended is: by default Stackube will run most of your workloads in a VM-based Pod, in this case directory sharing is used by hypervisor based runtime for volumes mounts, but this actually has more I/O overhead than bind mount. 

On the other hand, the hypervisor Pod make it possible to mount block device directly to the VM-based Pod, so we can eliminates directory sharing.

In Stackube, we use a flexvolume to directly use Cinder RBD based block device as Pod volume. The usage is very simple:

::

  apiVersion: v1
  kind: Pod
  metadata:
    name: web
    labels:
      app: nginx
  spec:
    containers:
    - name: nginx
      image: nginx
      ports:
      - containerPort: 80
      volumeMounts:
      - name: nginx-persistent-storage
        mountPath: /var/lib/nginx
    volumes:
    - name: nginx-persistent-storage
      flexVolume:
        driver: "cinder/flexvolume_driver"
        fsType: ext4
        options:
          cinderConfig: /etc/kubernetes/cinder.conf
          volumeID: daa7b4e6-1792-462d-ad47-78e900fed429

Please note the name of flexvolume is: "cinder/flexvolume_driver". Users are expected to provide a valid volume ID created with Cinder beforehand. Then a related RBD device will be attached to the VM-based Pod.

If your cluster is installed by stackube/devstack or following other stackube official guide, a /etc/kubernetes/cinder.conf file will be generated automatically on every node. 

Otherwise, users are expected to write a /etc/kubernetes/cinder.conf on every node. The contents is like:

::

  [Global]
  auth-url = _AUTH_URL_
  username = _USERNAME_
  password = _PASSWORD_
  tenant-name = _TENANT_TNAME_
  region = _REGION_
  [RBD]
  keyring = _KEYRING_


and also, users need to make sure flexvolume_driver binary is in /usr/libexec/kubernetes/kubelet-plugins/volume/exec/cinder~flexvolume_driver/ of every node.

