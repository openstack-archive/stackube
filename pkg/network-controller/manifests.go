/*
Copyright (c) 2017 OpenStack Foundation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package network

const (
	kubeDNSDeployment = `
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    k8s-app: kube-dns
  name: kube-dns
  namespace: {{ .Namespace }}
spec:
  replicas: 1
  selector:
    matchLabels:
      k8s-app: kube-dns
  strategy:
    rollingUpdate:
      maxSurge: 10%
      maxUnavailable: 0
    type: RollingUpdate
  template:
    metadata:
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ""
      labels:
        k8s-app: kube-dns
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: beta.kubernetes.io/arch
                operator: In
                values:
                - amd64
      containers:
      - args:
        - --domain={{ .DNSDomain }}.
        - --dns-port=10053
        - --namespace=$(POD_NAMESPACE)
        - --config-dir=/kube-dns-config
        - --v=2
        env:
        - name: PROMETHEUS_PORT
          value: "10055"
        - name: KUBERNETES_SERVICE_HOST
          value: "{{ .KubernetesHost }}"
        - name: KUBERNETES_SERVICE_PORT
          value: "{{ .KubernetesPort }}"
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        image: {{ .KubeDNSImage }}
        imagePullPolicy: IfNotPresent
        name: kubedns
        ports:
        - containerPort: 10053
          name: dns-local
          protocol: UDP
        - containerPort: 10053
          name: dns-tcp-local
          protocol: TCP
        - containerPort: 10055
          name: metrics
          protocol: TCP
        resources:
          limits:
            cpu: 100m
            memory: 64Mi
        volumeMounts:
        - mountPath: /kube-dns-config
          name: kube-dns-config
      - args:
        - -v=2
        - -logtostderr
        - -configDir=/etc/k8s/dns/dnsmasq-nanny
        - -restartDnsmasq=true
        - --
        - -k
        - --cache-size=1000
        - --log-facility=-
        - --server=/{{ .DNSDomain }}/127.0.0.1#10053
        - --server=/in-addr.arpa/127.0.0.1#10053
        - --server=/ip6.arpa/127.0.0.1#10053
        image: {{ .DNSMasqImage }}
        imagePullPolicy: IfNotPresent
        name: dnsmasq
        ports:
        - containerPort: 53
          name: dns
          protocol: UDP
        - containerPort: 53
          name: dns-tcp
          protocol: TCP
        resources:
          limits:
            cpu: 150m
            memory: 64Mi
        volumeMounts:
        - mountPath: /etc/k8s/dns/dnsmasq-nanny
          name: kube-dns-config
      - command:
        - /bin/sh
        - -c
        - /sidecar --v=2 --logtostderr --probe=kubedns,127.0.0.1:10053,$(hostname -i | tr '.' '-').$(POD_NAMESPACE).pod.{{ .DNSDomain }},5,A --probe=dnsmasq,127.0.0.1:53,$(hostname -i | tr '.' '-').$(POD_NAMESPACE).pod.{{ .DNSDomain }},5,A
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        image: {{ .SidecarImage }}
        imagePullPolicy: IfNotPresent
        name: sidecar
        ports:
        - containerPort: 10054
          name: metrics
          protocol: TCP
        resources:
          limits:
            cpu: 150m
            memory: 128Mi
          requests:
            cpu: 10m
            memory: 20Mi
      dnsPolicy: Default
      restartPolicy: Always
      terminationGracePeriodSeconds: 30
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
      - key: CriticalAddonsOnly
        operator: Exists
      volumes:
      - configMap:
          defaultMode: 420
          name: kube-dns
          optional: true
        name: kube-dns-config
`

	kubeDNSService = `
apiVersion: v1
kind: Service
metadata:
  labels:
    k8s-app: kube-dns
    kubernetes.io/name: KubeDNS
  name: kube-dns
  namespace: {{ .Namespace }}
spec:
  ports:
  - name: dns
    port: 53
    protocol: UDP
    targetPort: 53
  - name: dns-tcp
    port: 53
    protocol: TCP
    targetPort: 53
  selector:
    k8s-app: kube-dns
  sessionAffinity: None
  type: ClusterIP
`
)
