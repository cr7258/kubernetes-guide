## 创建 Linux 虚拟机

在启动的虚拟机中会按照 Docker, Kind, Kubectl 等工具，我的电脑安装的是 ARM 架构的，如果是 X86 架构的电脑，需要修改 vm.yaml 文件中相关的安装命令。

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

## 第一章 Kubelet 快速魔改，本地启动
### 修改 Kubelet 代码

我们需要修改 Kubelet 中的 Cadvisor（容器监控），CRI 交互代码（ImageService, RuntimeService）以模拟假的节点。

### 启动 Kubelet

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

### 节点 Ready 状态的原理

Kubernetes 节点发送的心跳帮助你的集群确定每个节点的可用性，并在检测到故障时采取行动。

对于节点，有两种形式的心跳:
- 更新节点的 .status
- kube-node-lease 名字空间中的 Lease（租约）对象。 每个节点都有一个关联的 Lease 对象。
与 Node 的 .status 更新相比，Lease 是一种轻量级资源。 使用 Lease 来表达心跳在大型集群中可以减少这些更新对性能的影响。

kubelet 负责创建和更新节点的 .status，以及更新它们对应的 Lease。
- 当节点状态发生变化时，或者在配置的时间间隔内没有更新事件时，kubelet 会更新 .status。 .status 更新的默认间隔为 5 分钟（比节点不可达事件的 40 秒默认超时时间长很多）。
- kubelet 会创建并每 10 秒（默认更新间隔时间）更新 Lease 对象。 Lease 的更新独立于 Node 的 .status 更新而发生。 如果 Lease 的更新操作失败，kubelet 会采用指数回退机制，从 200 毫秒开始重试， 最长重试间隔为 7 秒钟。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230520113851.png)


### 模拟 Kubelet Lease 续期

当我们停止本地的 Kubelet 时，等待 40 秒后，因为 Lease 没有被及时更新，节点状态会变为 NotReady。

```bash
root@lima-vm:~# kubectl get node
NAME                         STATUS     ROLES           AGE   VERSION
kubelet-demo-control-plane   Ready      control-plane   42m   v1.26.3
myjtthink                    NotReady   <none>          19m   v1.22.15
```

启动程序模拟 Kubelet Lease 续期，并将节点状态改为 Ready。

```bash
cd kubernetes-1.22.15/mykubelet/test
go run lease.go
```

查看节点状态，此时节点状态变为 Ready。

```bash
root@lima-vm:~# kubectl get node
NAME                         STATUS   ROLES           AGE   VERSION
kubelet-demo-control-plane   Ready    control-plane   96m   v1.26.3
myjtthink                    Ready    <none>          72m   v1.22.15
```

## 第二章 代码实现 Kubelet 注册(TLS Bootstrap)