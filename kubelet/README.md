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

TLS 启动引导机制：https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/kubelet-tls-bootstrapping/
- 1.kubelet 先使用一个预先商定好的低权限 token 连接到 kube-apiserver。 
- 2.向 kube-apiserver 申请证书，然后 kube-controller-manager 给 kubelet 动态签署证书（包括手动批准 CSR）。 
- 3.后续 kubelet 都将通过动态签署的证书与 kube-apiserver 通信。

执行以下命令用我们的代码创建 Token 以及 Secert。

```bash
cd kubernetes-1.22.15/mykubelet/test
go run token.go

# 输出
secret 创建成功: bootstrap-token-o0phpg
```

这个 token 创建后权限来自于 `system:node-bootstrapper` ClusterRole 中，Node Bootstrap Token 属于 `system:bootstrappers:kubeadm:default-node-token` 组。当我们使用 `kubeadm init` 命令时，这个东西就会被自动初始化。
文档说明：https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/kubelet-tls-bootstrapping/#authorize-kubelet-to-create-csr

```yaml
root@lima-vm:~# kubectl get clusterrolebinding kubeadm:kubelet-bootstrap -o yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  creationTimestamp: "2023-05-20T02:54:32Z"
  name: kubeadm:kubelet-bootstrap
  resourceVersion: "234"
  uid: 441f8e3c-6805-40f1-b251-67ef7d788465
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:node-bootstrapper
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: Group
  name: system:bootstrappers:kubeadm:default-node-token
```

kubelet 此时拥有受限制的凭据来创建和取回证书签名请求（CSR）。

```yaml
root@lima-vm:~# kubectl get clusterrole system:node-bootstrapper -n kube-system -o yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  annotations:
    rbac.authorization.kubernetes.io/autoupdate: "true"
  creationTimestamp: "2023-05-20T02:54:30Z"
  labels:
    kubernetes.io/bootstrapping: rbac-defaults
  name: system:node-bootstrapper
  resourceVersion: "87"
  uid: 13060586-e5a0-4356-8e88-dfa4ae8415b8
rules:
- apiGroups:
  - certificates.k8s.io
  resources:
  - certificatesigningrequests
  verbs:
  - create
  - get
  - list
  - watch
```

## 手工实现 CSR 请求和获取证书

### 1 创建 CSR 文件

```bash
# CN 是用户名，O 是该用户归属的组
openssl genrsa -out test.key 2048  
openssl req -new -key test.key -out test.csr -subj "/O=system:nodes/CN=system:node:chengzw"
```

### 2 创建 CertificateSigningRequest 对象

```yaml
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: testcsr
spec:
  # 把 CSR 文件的内容贴进去
  request: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURSBSRVFVRVNULS0tLS0KTUlJQ2VUQ0NBV0VDQVFBd05ERVZNQk1HQTFVRUNnd01jM2x6ZEdWdE9tNXZaR1Z6TVJzd0dRWURWUVFEREJKegplWE4wWlcwNmJtOWtaVHB6YUdWdWVXa3dnZ0VpTUEwR0NTcUdTSWIzRFFFQkFRVUFBNElCRHdBd2dnRUtBb0lCCkFRREJLRmRsMnp4KzJlbXRXWlBJYThTaXAwSkVHT3hUM0swK1I5M2JxdFJvTzNNS2lFazVwd0g5Z2V5Y2dqWXAKL0dSTnpQb2dVSnlWSU0veWJqRHF2a0Z2VXNIL2Mwc3ZJcVJ5Wk1GYXUxQ01ZMTU5cTNzV1dvQ0FlVEdCZFIzWQpkQXJZRnhsL1dNN3F6cmlaWVVrYzFudEs4QldtSjN4MjRWdkxDUHp5RVhjTjZLOTFCVm44bk05MWxncnJINFU3CndFWFVsS1VVeG1PU24vQzZnNUtlZ2I2cUlwdi8vaE1vUjhZMEowelVZenc5VkhiQXRMWkYwalF4Mi9QS0lDYVgKU1VDdk1UaGp0Q0FScTAxUk5sNWswaXdFZjh1NW94aEpqaDNMN2V0ZHRSdU96NzFrWktLUmg4bFhXWVp3YzRDNQphRGdjQmZjd2ZHQjdPVGhpNmhMN2JFVzNBZ01CQUFHZ0FEQU5CZ2txaGtpRzl3MEJBUXNGQUFPQ0FRRUFNT2lBCjZoNzlzODlGVytydUhvNEEvOTE3em1WZ0tPZXYremhnMDRaMzkwN0IwdmhzTUNvdTluckxEM0pyclVIMTYyOGQKd1JOclJuUWJObnNXVVhqNmtuUkJRYVQxSHZua2lkbEFDc0t6d2drQmFMOG80TEZxZUxRWTAyWVNDeVdvWVlCaQpGTm56OVVrbkQwcGcxU21DTEIrZ0pybGEwZ3IwTmloRk55dnN6YkY0a0lKamhFUnUvVVVxZWFKVnNDc2M5TDBkCmVsUVNmSkZ4OFRZVjQ5cWIremtQd3UySmlobEh6Ny96bTJKK0hnUVZtMkt0Ull1elNRN2FOWThDZElQa0kzZGQKVjRHd3g0Y3lIRU5wcmtvUXArVis4Vlp2QUZXL3I0aE9EWmswOXdBbXh6aFR3b2ora080RWtWdmozeEFZS0FFSQowdDBNem40WkMyNzdUbzFyaGc9PQotLS0tLUVORCBDRVJUSUZJQ0FURSBSRVFVRVNULS0tLS0K
  signerName: kubernetes.io/kube-apiserver-client
  expirationSeconds: 3600
  usages:
    - client auth
```

### 3 手动批复

```bash
kubectl certificate approve testcsr
```

### 4 获取证书内容

```bash
kubectl get csr  testcsr  -o jsonpath='{.status.certificate}'| base64 -d > testcsr .crt
```

## 代码实现 CSR 请求

执行以下代码：会在 Kubernetes 集群中创建 CertificateSigningRequest 对象，并将 Private Key 保存到 kubelet.key 文件中。

```bash
cd kubernetes-1.22.15/mykubelet/test
go run create_csr.go
```

执行以下命令手动批准 CSR。

```bash
kubectl certificate approve testcsr
```

代码会从 Kubernetes 集群中获取证书内容，并将其保存到 kubelet.pem 文件中。

```yaml
root@lima-vm:~# kubectl get csr testcsr -o yaml
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  creationTimestamp: "2023-05-21T07:01:34Z"
  name: testcsr
  resourceVersion: "134931"
  uid: 181f646b-e1e4-4d3e-9fde-0e6c8a738be0
spec:
  expirationSeconds: 36000
  groups:
    - system:masters
    - system:authenticated
  request: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURSBSRVFVRVNULS0tLS0KTUlIdk1JR1hBZ0VBTURVeEZUQVRCZ05WQkFvVERITjVjM1JsYlRwdWIyUmxjekVjTUJvR0ExVUVBeE1UYzNsegpkR1Z0T201dlpHVTZZMmhsYm1kNmR6QlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDlBd0VIQTBJQUJPNitiZ3hUCkJmNjQ0TGxVVXNJMisrOVJZcWNCbW1JczhXWWlTOXhTN29yaVhEOC9WQmEwcVNZY3E1QkFkRk5VZDFGODQ1YWgKc2ZRNDhOZXU0cVlxZm02Z0FEQUtCZ2dxaGtqT1BRUURBZ05IQURCRUFpQTBxR2RXZ05vTGxkQy9Nd0JrVm1PcQpvaXR4ZURGTzRuNjRNekZZblRnRHNBSWdZMmZISk1WSy9tc3pmWHV0VU1qd1FnZ1RCYTJxbzV2SWRLYTJnb0FlCkt1OD0KLS0tLS1FTkQgQ0VSVElGSUNBVEUgUkVRVUVTVC0tLS0tCg==
  signerName: kubernetes.io/kube-apiserver-client
  usages:
    - client auth
  username: kubernetes-admin
status:
  certificate: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNVRENDQVRpZ0F3SUJBZ0lSQUpNcmJQTXRPeTVVYTAvaUdDcStIUVF3RFFZSktvWklodmNOQVFFTEJRQXcKRlRFVE1CRUdBMVVFQXhNS2EzVmlaWEp1WlhSbGN6QWVGdzB5TXpBMU1qRXdOalUyTXpsYUZ3MHlNekExTWpFeApOalUyTXpsYU1EVXhGVEFUQmdOVkJBb1RESE41YzNSbGJUcHViMlJsY3pFY01Cb0dBMVVFQXhNVGMzbHpkR1Z0Ck9tNXZaR1U2WTJobGJtZDZkekJaTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEEwSUFCTzYrYmd4VEJmNjQKNExsVVVzSTIrKzlSWXFjQm1tSXM4V1lpUzl4UzdvcmlYRDgvVkJhMHFTWWNxNUJBZEZOVWQxRjg0NWFoc2ZRNAo4TmV1NHFZcWZtNmpSakJFTUJNR0ExVWRKUVFNTUFvR0NDc0dBUVVGQndNQ01Bd0dBMVVkRXdFQi93UUNNQUF3Ckh3WURWUjBqQkJnd0ZvQVUyTjFkeEhueWJhcVQxa2c2ZVVHNmc3QW9zd3N3RFFZSktvWklodmNOQVFFTEJRQUQKZ2dFQkFEYTYzbHdJOWRCbFMwT1A4bjJ0cnhnY1RzRXdzY0J0SlBBOGZyMTNwNDA2ZzVkTTZSRFFYRDl1VHU2NAoyQ2VndERKNDJQeTR2aWNML3RsYURXVHBKZVdRZkR6S0MwOVFIeldZc2lpRHdpY1FOQjBXekphdi83UC9nakJrCi9yWksyM3NDRlFjVlRGNnRTRlNMTlA2aHczRDBZNER4TlE2WmhQZE5pSnN1eWJDUzN0UjJmdWlXdjFOcExJbGgKYTJpeGFRUjhZTjR5QVU5dEJuWGkzK3NWeW9nMnZzRUVBN1h6R2J1alNyaU1FNmV3enAzT1NqYm9vZGRsdS9LSQpqTnJmVGRQNEhVa0dCUnQvYTc5VjFudUNtMmtxVTFTclVSMHNqcFl2emtOTnNUa3o1TXRLZnpvZ2ZEQlBFRHMxCmRlelIyNmlwVlhDVDhyTDhmdWdZLzZjSzVBRT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
  conditions:
    - lastTransitionTime: "2023-05-21T07:01:39Z"
      lastUpdateTime: "2023-05-21T07:01:39Z"
      message: This CSR was approved by kubectl certificate approve.
      reason: KubectlApprove
      status: "True"
      type: Approved
```

验证签发证书的有效性，使用 kubectl --kubeconfig 指定 kubeconfig 文件，使用签发证书的用户身份访问集群。

```bash
cd kubernetes-1.22.15/mykubelet
kubectl --kubeconfig kubelet.config get nodes
```