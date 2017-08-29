# Setting Up A Multi-nodes Stackube (Without HA For Now)

This page describes how to setup a multi-nodes cluster of Stackube.

## Prerequisites

### Roles

A stackube deployment is comprised by four kinds of nodes: control, network, compute, storage.

- Control
    - The control node is where openstack/kubernetes/ceph's control-plane will run.
    - **At least one and only one node** (for now).
    - Minimum hardware requirements:
        - Two network interfaces
            - One is for public network connection, with a public IP.
            - The other one is for private network connection, with a private IP and MTU >= 1600.
        - 8GB main memory
        - 50GB disk space

- Network
    - The network nodes are where neutron l3/lbaas/dhcp agents will run.
    - At least one node.
    - Minimum hardware requirements:
        - Two network interfaces
            - One is as neutron-external-interface. Public IP is not needed.
            - The other one is for private network connection, with a private IP and MTU >= 1600.
        - 8GB main memory
        - 50GB disk space

- Compute
    - The compute nodes are where your workloads will run.
    - At least one node.
    - Minimum hardware requirements:
        - One network interface
            - For private network connection, with a private IP and MTU >= 1600.
        - 8GB main memory
        - 50GB disk space

- Storage
    - The storage nodes are where ceph-osd(s) will run.
    - At least one node.
    - Minimum hardware requirements:
        - One network interface
            - For private network connection, with a private IP and MTU >= 1600.
        - 8GB main memory
        - 50GB disk space

There is no conflict between any two roles. That means, all of the roles could be deployed on the same node(s).

### Host OS
For now only CentOS 7.x is supported.

### Public IP Pool
A number of public IPs are needed.


## Deploy

All instructions below **must be done on the control node.**

### 1. SSH To The Control Node, And Become Root 
```
sudo su -
```

### 2. Enable Password-Less SSH

The control node needs to ssh to all nodes when deploying.

- Generate SSH keys on the control node. Leave the passphrase empty:

```
ssh-keygen

Generating public/private rsa key pair.
Enter file in which to save the key (/root/.ssh/id_rsa): 
Enter passphrase (empty for no passphrase): 
Enter same passphrase again: 
Your identification has been saved in /root/.ssh/id_rsa.
Your public key has been saved in /root/.ssh/id_rsa.pub.
```

- Copy the key to each node (including the control node itself):
```
ssh-copy-id root@NODE_IP
```

### 3. Clone Stackube Repo
```
git clone https://git.openstack.org/openstack/stackube
```

### 4. Edit The Config File
```
cd stackube/install
vim config_example
```

### 5. Do The Deploy
```
bash deploy.sh config_example
```

If failed, please **do remove** (as shown below) before deploy again.



## Remove
```
bash remove.sh config_example
```
