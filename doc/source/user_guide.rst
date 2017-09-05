Stackube User Guide
=====================================

=============================
Tenant and Network Management
=============================

In this part, we will introduce tenant management and networking in Stackube. The tenant, which is ``1:1`` mapped with k8s namespace, is managed by using k8s CRD (previous TPR) to interact with Keystone. And the tenant is also ``1:1`` mapped with a network automatically, which is also implemented by CRD with standalone Neutron.

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

4. Check the ``kube-dns`` pods created in the new namespace.

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



=============================
Persistent volume
=============================

This part describes the persistent volume design and usage in Stackube.

=================
Standard Kubernetes volume
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

Please note since Stackube is a baremetal k8s cluster, cloud provider based volume are not supported by default.

But unless you are using ``emptyDir`` or ``hostPath``, we will recommend always you the ``Cinder RBD based block device as volume`` described below in Stackube, this will bring you much higher performance.

==================================
Cinder RBD based block device as volume
==================================

The reason this volume type is preferred is: by default Stackube will run most of your workloads in a VM-based Pod, in this case directory sharing is used by hypervisor based runtime for volumes mounts, but this actually has more I/O overhead than bind mount. 

On the other hand, the hypervisor Pod make it possible to mount block device directly to the VM-based Pod, so we can eliminates directory sharing.

In Stackube, we use a flexvolume to directly use Cinder RBD based block device as Pod volume. The usage is very simple:

1. Create a Cinder volume (skip if you want to use a existing Cinder volume).

::

  $ cinder create --name volume 1

2. Create a Pod claim to use this Cinder volume.

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
          volumeID: daa7b4e6-1792-462d-ad47-78e900fed429

Please note the name of flexvolume should be: ``cinder/flexvolume_driver``. 

The ``daa7b4e6-1792-462d-ad47-78e900fed429`` is either volume ID created with Cinder or any existing available Cinder volume ID. After this yaml is applied, the related RBD device will be attached to the VM-based Pod after this is created.

========
Others
========

If your cluster is installed by ``stackube/devstack`` or following other stackube official guide, a ``/etc/kubernetes/cinder.conf`` file will be generated automatically on every node. Otherwise, you are expected to create a ``/etc/kubernetes/cinder.conf`` on every node. The contents is like:

::

  [Global]
  auth-url = _AUTH_URL_
  username = _USERNAME_
  password = _PASSWORD_
  tenant-name = _TENANT_NAME_
  region = _REGION_
  [RBD]
  keyring = _KEYRING_


and also, users need to make sure flexvolume_driver binary is in ``/usr/libexec/kubernetes/kubelet-plugins/volume/exec/cinder~flexvolume_driver/`` of every node.
