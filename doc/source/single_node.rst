Setting up a single node Stackube
=====================================

This page describes how to setup a working development environment that can be used in developing stackube on Ubuntu or CentOS. These instructions assume you're already installed git, golang and python on your host.

=================
Getting the code
=================

Grab the code:
::

  git clone https://git.openstack.org/openstack/stackube

==================================
Spawn up Kubernetes and OpenStack
==================================

devstack is used to spawn up a kubernetes and openstack environment.

Create stack user:
::

  sudo useradd -s /bin/bash -d /opt/stack -m stack
  echo "stack ALL=(ALL) NOPASSWD: ALL" | sudo tee /etc/sudoers.d/stack
  sudo su - stack

Grab the devstack:
::

  git clone https://git.openstack.org/openstack-dev/devstack -b stable/ocata
  cd devstack

Create a local.conf:
::

  curl -sSL https://raw.githubusercontent.com/openstack/stackube/master/devstack/local.conf.sample -o local.conf

Start installation:
::

  ./stack.sh

Setup environment variables for kubectl and openstack client:
::

  export KUBECONFIG=/opt/stack/admin.conf
  source /opt/stack/devstack/openrc admin admin

Setup environment variables for kubectl and openstack client:
::

  export KUBECONFIG=/etc/kubernetes/admin.conf 
  source openrc admin admin
