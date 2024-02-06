# VirtNet

VirtNet 提供完备的 Kubernetes Underlay 网络解决方案，区别于基于 Veth 虚拟网卡的 CNI 解决方案，Underlay 网络数据包避免了宿主机的三层网络转发，没有隧道封装开销。

VirtNet 支持为 Deployment、StatefulSet、DaemonSet 和 VirtualMachine 的 Pod 分配 IP 地址，支持 StatefulSet 和 VirtualMachine Pod 的固定 IP 分配。

## Quick start

使用 Helm 安装 VirtNet:

```bash
helm install virtnet -n kube-system oci://registry-1.docker.io/hejianmin/chart-virtnet --version v0.2.11-dev
```

等待 virtnet-agent 和 virtnet-ctrl ready:

```bash
[root@centos7 virtnet]# k get pod -n kube-system | grep virtnet
virtnet-agent-cjk8w                       1/1     Running   0               60s
virtnet-ctrl-db6559686-7hzml              1/1     Running   0               60s
```

将 VirtNet 设置为集群默认 CNI 插件:

```bash
# 在各个 Node 上操作
# plugins.master 替换为 Node 网卡的名称，例如 ens192
# plugins.ipam.default_subnet 替换为 Subnet CR 名称，例如 subnet
vim /etc/cni/net.d/00-virtnet.conflist
{
  "cniVersion": "0.3.1",
  "name": "macvlan",
  "plugins": [
    {
      "type": "macvlan",
      "master": "ens192",
      "mode": "bridge",
      "ipam": {
        "type": "virtnet-ipam",
        "default_subnet": "subnet"
      }
    },
    {
      "type": "virtnet-coordinator"
    }
  ]
}
```

创建 Subnet CR:

```bash
apiVersion: virtnet.io/v1alpha1
kind: Subnet
metadata:
  name: subnet
spec:
  replicas: 3
  gateway: 10.6.0.1
  ips:
    - 10.6.153.100-10.6.153.255
  subnet: 10.6.0.0/16
```

查看 Subnet 的 IPPool 是否自动创建成功：

```bash
[root@centos7 virtnet]# k get subnets.virtnet.io
NAME     GATEWAY    SUBNET        REPLICAS   TOTAL-IP-COUNT
subnet   10.6.0.1   10.6.0.0/16   3          156
[root@centos7 virtnet]# k get ippools.virtnet.io
NAME           GATEWAY    SUBNET        ALLOCATED-IP-COUNT   UNALLOCATED-IP-COUNT   TOTAL-IP-COUNT
subnet-fs95z   10.6.0.1   10.6.0.0/16                        52                     52
subnet-gfrd7   10.6.0.1   10.6.0.0/16                        52                     52
subnet-mm8f9   10.6.0.1   10.6.0.0/16                        52                     52
```

查看 Node 和 coredns 是否 ready:

```bash
[root@centos7 virtnet]# k get node
NAME      STATUS   ROLES                  AGE   VERSION
centos7   Ready    control-plane,worker   48d   v1.25.3
[root@centos7 virtnet]# k get pod -n kube-system | grep coredns
coredns-675f76bb78-6j7h7                  1/1     Running   0               26s
```

创建 Pod，验证是否自动分配 IP：

```bash
[root@centos7 virtnet]# k get subnets.virtnet.io
NAME     GATEWAY    SUBNET        REPLICAS   TOTAL-IP-COUNT
subnet   10.6.0.1   10.6.0.0/16   3          156
[root@centos7 virtnet]# k get ippools.virtnet.io
NAME           GATEWAY    SUBNET        ALLOCATED-IP-COUNT   UNALLOCATED-IP-COUNT   TOTAL-IP-COUNT
subnet-fs95z   10.6.0.1   10.6.0.0/16   2                    50                     52
subnet-gfrd7   10.6.0.1   10.6.0.0/16   3                    49                     52
subnet-mm8f9   10.6.0.1   10.6.0.0/16   3                    49                     52
[root@centos7 virtnet]# k get endpoints.virtnet.io
NAME                      INTERFACE   IPV4POOL       IPV4              OWNER-KIND    OWNER-NAME          NODE
sample-78b779bcc7-d29fb   eth0        subnet-fs95z   10.6.153.101/16   ReplicaSet    sample-78b779bcc7   centos7
sample-78b779bcc7-gd8qk   eth0        subnet-mm8f9   10.6.153.205/16   ReplicaSet    sample-78b779bcc7   centos7
sample-78b779bcc7-xfzl6   eth0        subnet-gfrd7   10.6.153.154/16   ReplicaSet    sample-78b779bcc7   centos7
sample-ss-0               eth0        subnet-fs95z   10.6.153.104/16   StatefulSet   sample-ss           centos7
sample-ss-1               eth0        subnet-gfrd7   10.6.153.157/16   StatefulSet   sample-ss           centos7
sample-ss-2               eth0        subnet-mm8f9   10.6.153.209/16   StatefulSet   sample-ss           centos7
sample-ss-3               eth0        subnet-fs95z   10.6.153.105/16   StatefulSet   sample-ss           centos7
sample-ss-4               eth0        subnet-gfrd7   10.6.153.158/16   StatefulSet   sample-ss           centos7
[root@centos7 virtnet]# k get pod -o wide
NAME                      READY   STATUS    RESTARTS   AGE     IP             NODE      NOMINATED NODE   READINESS GATES
sample-78b779bcc7-d29fb   1/1     Running   0          6m22s   10.6.153.101   centos7   <none>           <none>
sample-78b779bcc7-gd8qk   1/1     Running   0          6m22s   10.6.153.205   centos7   <none>           <none>
sample-78b779bcc7-xfzl6   1/1     Running   0          6m22s   10.6.153.154   centos7   <none>           <none>
sample-ss-0               1/1     Running   0          33s   10.6.153.104   centos7   <none>           <none>
sample-ss-1               1/1     Running   0          32s   10.6.153.157   centos7   <none>           <none>
sample-ss-2               1/1     Running   0          31s   10.6.153.209   centos7   <none>           <none>
sample-ss-3               1/1     Running   0          30s   10.6.153.105   centos7   <none>           <none>
sample-ss-4               1/1     Running   0          29s   10.6.153.158   centos7   <none>           <none>
```

重启 StatefulSet 的 Pod，发现 Pod 的 IP 地址没有发生改变。

```bash
[root@centos7 virtnet]# k get statefulset
NAME        READY   AGE
sample-ss   5/5     3m20s
[root@centos7 virtnet]# k rollout restart statefulset sample-ss
statefulset.apps/sample-ss restarted
[root@centos7 virtnet]# k get pod -o wide -w
NAME                      READY   STATUS    RESTARTS   AGE     IP             NODE      NOMINATED NODE   READINESS GATES
sample-78b779bcc7-d29fb   1/1     Running   0          14m     10.6.153.101   centos7   <none>           <none>
sample-78b779bcc7-gd8qk   1/1     Running   0          14m     10.6.153.205   centos7   <none>           <none>
sample-78b779bcc7-xfzl6   1/1     Running   0          14m     10.6.153.154   centos7   <none>           <none>
sample-ss-0               1/1     Running   0          9s      10.6.153.104   centos7   <none>           <none>
sample-ss-1               1/1     Running   0          42s     10.6.153.157   centos7   <none>           <none>
sample-ss-2               1/1     Running   0          74s     10.6.153.209   centos7   <none>           <none>
sample-ss-3               1/1     Running   0          106s    10.6.153.105   centos7   <none>           <none>
sample-ss-4               1/1     Running   0          2m19s   10.6.153.158   centos7   <none>           <none>
```

