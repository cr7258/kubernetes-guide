## 创建 Linux 虚拟机

在启动的虚拟机中会按照 Docker, Kind, Kubectl 等工具，我的电脑安装的是 ARM 架构的。

```bash
# 启动虚拟机
limactl start vm.yaml

# 进入虚拟机
limactl shell vm 
```

## 使用 Kind 创建 Kubernetes 集群

```bash
kind create cluster --name kubelet-demo
```

## 修改 Kubelet 代码

我们需要修改 Kubelet 中的 Cadvisor（容器监控），CRI 交互代码（ImageService, RuntimeService）以模拟假的节点。

## 启动 Kubelet

```bash
sh boot.sh
```

确认本地启动的 Kubelet 节点已经成功注册到集群中。

```bash
root@lima-vm:~# kubectl  get node
NAME                         STATUS   ROLES           AGE   VERSION
kubelet-demo-control-plane   Ready    control-plane   23m   v1.26.3
# 我们本地启动的假节点
myjtthink                    Ready    <none>          16s   v1.22.15
```

查看假节点信息。

```yaml
root@lima-vm:~# kubectl describe node myjtthink
Name:               myjtthink
Roles:              <none>
Labels:             beta.kubernetes.io/arch=arm64
  beta.kubernetes.io/os=linux
  kubernetes.io/arch=arm64
  kubernetes.io/hostname=jtthink
  kubernetes.io/os=linux
  type=agent
Annotations:        node.alpha.kubernetes.io/ttl: 0
                      volumes.kubernetes.io/controller-managed-attach-detach: true
CreationTimestamp:  Sat, 20 May 2023 03:17:57 +0000
Taints:             <none>
Unschedulable:      false
Lease:
  HolderIdentity:  myjtthink
  AcquireTime:     <unset>
  RenewTime:       Sat, 20 May 2023 03:23:33 +0000
Conditions:
  Type             Status  LastHeartbeatTime                 LastTransitionTime                Reason                       Message
  ----             ------  -----------------                 ------------------                ------                       -------
  MemoryPressure   False   Sat, 20 May 2023 03:23:07 +0000   Sat, 20 May 2023 03:17:57 +0000   KubeletHasSufficientMemory   kubelet has sufficient memory available
  DiskPressure     False   Sat, 20 May 2023 03:23:07 +0000   Sat, 20 May 2023 03:17:57 +0000   KubeletHasNoDiskPressure     kubelet has no disk pressure
  PIDPressure      False   Sat, 20 May 2023 03:23:07 +0000   Sat, 20 May 2023 03:17:57 +0000   KubeletHasSufficientPID      kubelet has sufficient PID available
  Ready            True    Sat, 20 May 2023 03:23:07 +0000   Sat, 20 May 2023 03:18:07 +0000   KubeletReady                 kubelet is posting ready status. AppArmor enabled
Addresses:
  InternalIP:  192.168.5.15
  Hostname:    myjtthink
Capacity:
  cpu:                100
  ephemeral-storage:  0
  memory:             32Gi
  pods:               110
Allocatable:
  cpu:                100
  ephemeral-storage:  0
  memory:             32668Mi
  pods:               110
System Info:
  Machine ID:
  System UUID:
  Boot ID:
  Kernel Version:             3.10
  OS Image:
  Operating System:           linux
  Architecture:               arm64
  Container Runtime Version:  jtthink://Unknown
  Kubelet Version:            v1.22.15
  Kube-Proxy Version:         v1.22.15
PodCIDR:                      10.244.1.0/24
PodCIDRs:                     10.244.1.0/24
Non-terminated Pods:          (2 in total)
  Namespace                   Name                CPU Requests  CPU Limits  Memory Requests  Memory Limits  Age
  ---------                   ----                ------------  ----------  ---------------  -------------  ---
  kube-system                 kindnet-d2tcq       100m (0%)     100m (0%)   50Mi (0%)        50Mi (0%)      5m38s
  kube-system                 kube-proxy-27ckl    0 (0%)        0 (0%)      0 (0%)           0 (0%)         5m38s
Allocated resources:
  (Total limits may be over 100 percent, i.e., overcommitted.)
  Resource           Requests   Limits
  --------           --------   ------
  cpu                100m (0%)  100m (0%)
  memory             50Mi (0%)  50Mi (0%)
  ephemeral-storage  0 (0%)     0 (0%)
Events:
  Type    Reason                   Age                    From             Message
  ----    ------                   ----                   ----             -------
  Normal  Starting                 5m38s                  kubelet          Starting kubelet.
  Normal  NodeHasSufficientMemory  5m38s (x2 over 5m38s)  kubelet          Node myjtthink status is now: NodeHasSufficientMemory
  Normal  NodeHasNoDiskPressure    5m38s (x2 over 5m38s)  kubelet          Node myjtthink status is now: NodeHasNoDiskPressure
  Normal  NodeHasSufficientPID     5m38s (x2 over 5m38s)  kubelet          Node myjtthink status is now: NodeHasSufficientPID
  Normal  RegisteredNode           5m35s                  node-controller  Node myjtthink event: Registered Node myjtthink in Controller
  Normal  NodeReady                5m28s                  kubelet          Node myjtthink status is now: NodeReady
```