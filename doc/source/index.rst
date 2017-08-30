=============================================
Welcome to Stackube documentation!
=============================================

Stackube is a Kubernetes-centric OpenStack distro. It uses Kubernetes, instead of Nova, as the compute
fabric controller, to provision containers as the compute instance, along with other OpenStack
services (e.g. Cinder, Neutron). It supports multiple container runtime technologies, e.g. Docker,
Hyper, and offers built-in soft / hard multi-tenancy (depending on the container runtime used).

Stackube Authors
==============

Stackube is an open source project with an active development community. The project is initiated by HyperHQ, and involves contribution from ARM, China Mobile, etc.

Introduction
==============

.. toctree::
   :maxdepth: 2

   architecture

   stackube_scope_clarification

Deployment Guide
================

.. toctree::
   :maxdepth: 2

   setup

Developer Guide
================

.. toctree::
   :maxdepth: 2

   developer

User Guide
================

.. toctree::
   :maxdepth: 2

   user_guide

Release Note
================

.. toctree::
   :maxdepth: 2

   release_notes/1_0
