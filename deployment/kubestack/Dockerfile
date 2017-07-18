FROM busybox

MAINTAINER mozhuli <21621232@zju.edu.cn>

ADD _output/kubestack /opt/cni/bin/kubestack
ADD deployment/kubestack/install-cni.sh /install-cni.sh
ADD deployment/kubestack/10-kubestack.conf /etc/cni/net.d/10-kubestack.conf

ENV PATH=$PATH:/opt/cni/bin
VOLUME /opt/cni
WORKDIR /opt/cni/bin
CMD ["/opt/cni/bin/kubestack"]
