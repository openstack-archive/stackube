FROM alpine:3.6

MAINTAINER stackube team

RUN apk --no-cache add bash
ADD kubestack /opt/cni/bin/kubestack
ADD install-cni.sh /install-cni.sh
ADD 10-kubestack.conf /etc/cni/net.d/10-kubestack.conf
ADD kubestack.conf.default /kubestack.conf.tmp

ENV PATH=$PATH:/opt/cni/bin
VOLUME /opt/cni
WORKDIR /opt/cni/bin
