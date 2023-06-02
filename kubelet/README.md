* [环境准备](#环境准备)
   * [创建 Linux 虚拟机](#创建-linux-虚拟机)
   * [使用 Kind 创建 Kubernetes 集群](#使用-kind-创建-kubernetes-集群)
* [第一章 Kubelet 快速魔改，本地启动](#第一章-kubelet-快速魔改本地启动)
   * [修改 Kubelet 代码](#修改-kubelet-代码)
   * [启动 Kubelet](#启动-kubelet)
   * [节点 Ready 状态的原理](#节点-ready-状态的原理)
   * [模拟 Kubelet Lease 续期](#模拟-kubelet-lease-续期)
* [第二章 代码实现 Kubelet 注册(TLS Bootstrap)](#第二章-代码实现-kubelet-注册tls-bootstrap)
   * [手工实现 CSR 请求和获取证书](#手工实现-csr-请求和获取证书)
      * [1 创建 CSR 文件](#1-创建-csr-文件)
      * [2 创建 CertificateSigningRequest 对象](#2-创建-certificatesigningrequest-对象)
      * [3 手动批复](#3-手动批复)
      * [4 获取证书内容](#4-获取证书内容)
   * [代码实现 CSR 请求](#代码实现-csr-请求)
   * [手撸 Kubelet 之创建节点](#手撸-kubelet-之创建节点)
* [第三章 Pod 状态和监听（主模块源码学习）](#第三章-pod-状态和监听主模块源码学习)
   * [手动调用 PLEG](#手动调用-pleg)
   * [SyncLoop](#syncloop)
   * [手工调用 PodManager，创建一个假的静态 Pod](#手工调用-podmanager创建一个假的静态-pod)
   * [监听 Pod 加入缓存](#监听-pod-加入缓存)
   * [PodWorkers](#podworkers)


## 环境准备
### 创建 Linux 虚拟机

在启动的虚拟机中会按照 Docker, Kind, Kubectl 等工具，我的电脑安装的是 ARM 架构的，如果是 X86 架构的电脑，需要修改 vm.yaml 文件中相关的安装命令。

```bash
# 启动虚拟机
limactl start vm.yaml

# 进入虚拟机
limactl shell vm 
```

### 使用 Kind 创建 Kubernetes 集群

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

### 手工实现 CSR 请求和获取证书

#### 1 创建 CSR 文件

```bash
# CN 是用户名，O 是该用户归属的组
openssl genrsa -out test.key 2048  
openssl req -new -key test.key -out test.csr -subj "/O=system:nodes/CN=system:node:chengzw"
```

#### 2 创建 CertificateSigningRequest 对象

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

#### 3 手动批复

```bash
kubectl certificate approve testcsr
```

#### 4 获取证书内容

```bash
kubectl get csr  testcsr  -o jsonpath='{.status.certificate}'| base64 -d > testcsr .crt
```

### 代码实现 CSR 请求

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

### 手撸 Kubelet 之创建节点

进入 mykubelet-demo 目录启动程序。

```bash
root@lima-vm:/Users/I576375/Code/kubernetes-guide/kubelet/mykubelet-demo#  go run main.go 
I0521 09:42:28.541023   79863 bootstrap.go:17] begin bootstrap 
I0521 09:42:28.565105   79863 csr.go:106] waiting for csr is approved....

# 手动批准 CSR
kubectl certificate approve myk8s

# 输出
I0521 09:43:06.137346   79863 bootstrap.go:29] kubelet pem-files have been saved in .kube 
I0521 09:43:06.138625   79863 csr.go:159] writing kubelet-config to  ./.kube/kubelet.config
I0521 09:43:06.141958   79863 bootstrap.go:35] testing kubeclient
I0521 09:43:06.200443   79863 bootstrap.go:44] v1.26.3
I0521 09:43:06.216640   79863 node.go:35] create node myk8s success 
I0521 09:43:06.225844   79863 node.go:50]   node status update success 
# 开始持续续期
I0521 09:43:06.225891   79863 node_lease.go:59] starting lease controller
```

查看节点状态。

```bashroot@lima-vm:~# kubectl get node
NAME                         STATUS     ROLES           AGE    VERSION
kubelet-demo-control-plane   Ready      control-plane   30h    v1.26.3
myk8s                        Ready      <none>          105s   v1.22.99
```

## 第三章 Pod 状态和监听（主模块源码学习）

PLEG：全称 Pod Lifecycle Event Generator（Pod 生命周期事件生成器），它会定期检查节点上 Pod 的运行状态，把 Pod 的状态变化封装为特有的 Event（PodLifeCycleEvent），从而触发 kubelet 的主同步机制。

主要参考源码中的 GetPods 方法，相关代码在 kubernetes-1.22.15/mykubelet/mylib 目录下的 runtime.go, runtime_util.go, runtimeservice.go, runtimeservice_mock.go 文件中。

```go
func (m *kubeGenericRuntimeManager) GetPods(all bool) ([]*kubecontainer.Pod, error) {
	pods := make(map[kubetypes.UID]*kubecontainer.Pod)
	sandboxes, err := m.getKubeletSandboxes(all)
	if err != nil {
		return nil, err
	}
	for i := range sandboxes {
		s := sandboxes[i]
		if s.Metadata == nil {
			klog.V(4).InfoS("Sandbox does not have metadata", "sandbox", s)
			continue
		}
		podUID := kubetypes.UID(s.Metadata.Uid)
		if _, ok := pods[podUID]; !ok {
			pods[podUID] = &kubecontainer.Pod{
				ID:        podUID,
				Name:      s.Metadata.Name,
				Namespace: s.Metadata.Namespace,
			}
		}
		p := pods[podUID]
		converted, err := m.sandboxToKubeContainer(s)
		if err != nil {
			klog.V(4).InfoS("Convert sandbox of pod failed", "runtimeName", m.runtimeName, "sandbox", s, "podUID", podUID, "err", err)
			continue
		}
		p.Sandboxes = append(p.Sandboxes, converted)
	}

	containers, err := m.getKubeletContainers(all)
	if err != nil {
		return nil, err
	}
	for i := range containers {
		c := containers[i]
		if c.Metadata == nil {
			klog.V(4).InfoS("Container does not have metadata", "container", c)
			continue
		}

		labelledInfo := getContainerInfoFromLabels(c.Labels)
		pod, found := pods[labelledInfo.PodUID]
		if !found {
			pod = &kubecontainer.Pod{
				ID:        labelledInfo.PodUID,
				Name:      labelledInfo.PodName,
				Namespace: labelledInfo.PodNamespace,
			}
			pods[labelledInfo.PodUID] = pod
		}

		converted, err := m.toKubeContainer(c)
		if err != nil {
			klog.V(4).InfoS("Convert container of pod failed", "runtimeName", m.runtimeName, "container", c, "podUID", labelledInfo.PodUID, "err", err)
			continue
		}

		pod.Containers = append(pod.Containers, converted)
	}

	// Convert map to list.
	var result []*kubecontainer.Pod
	for _, pod := range pods {
		result = append(result, pod)
	}

	return result, nil
}
```

### 手动调用 PLEG

PLEG 通过 relist 函数获取 Pod 列表并存到本地缓存，然后定时再取，每次和之前的缓存比对，从而得知哪些 Pod 发生了变化。

```go
rs := &mylib.MyRuntimeService{} // CRI 模拟实现
// 模拟创建 kubelet 封装的 runtime
var cr kubecontainer.Runtime = mylib.NewContianerRuntime(rs, "containerd")
cache := kubecontainer.NewCache()
p := pleg.NewGenericPLEG(cr, 1000, time.Second*1, cache, clock.RealClock{})
go func() {
    for {
        select {
        case v := <-p.Watch():
            if v.Type != pleg.ContainerStarted {
                fmt.Println(v)
                break
            }
        }
    }
}()
p.Start()

// 启动 HTTP 服务，当收到请求时，将 Pod 状态改为 NotReady
http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
    mylib.MockData_Pods[0].State = runtimeapi.PodSandboxState_SANDBOX_NOTREADY
    writer.Write([]byte("Pod 状态变更"))
})

http.ListenAndServe(":8080", nil)
```

启动程序。

```bash
cd kubernetes-1.22.15/mykubelet/
go run mytest/myclient/pleg.go
```

浏览器输入 http://localhost:8080，得到以下内容。

```bash
Pod 状态变更
```

查看程序，输出以下内容。

```bash
&{ef14133d-c5af-482d-a514-e6fc98093553 ContainerDied 926f1b5a1d33a}
```

### SyncLoop

syncLoop 是处理变更的主循环，监听来自 file, http, API Server 的事件更新。syncLoopIteration 从各个 channel 中读取数据，并将 Pod 分派给给定的处理 handler。
- configCh: 监听 Pod 配置的变更
- plegCh: 监听来自 PLEG 的事件
- syncCh: 监听处理等待同步的 Pod
- housekeepingCh: 监听处理需要清理的 Pod
- health manager（livenessManager, readinessManager, startupManager）: 监听探针事件

statusManager 的主要功能是将 Pod 的状态信息同步到 API Server，它并不会主动监控 Pod 的状态，而是提供接口供其他 manager（例如 probeManager）进行调用，同时 syncLoop 主循环也会调用到它。Manager 接口包含以下几个主要方法：
- SetPodStatus：Pod 状态发生变化，会调用新状态更新到 API Server。
- SetContainerReadiness：Pod 中的容器健康状态发生变化，会调用修改 Pod 的监健康状态。
- TerminatePod：删除 Pod 的时候调用，把 Pod 中所有的容器设置为 terminated 状态。
- RemoveOrphanedStatuses：删除 Orphan Pod。


StatusManager 初始化（pkg/kubelet/status/status_manager.go，122 行）：
- kubeClient：用于和 API Server 交互
- podManager：Pod 内存形式的管理器，用于管理 Kubelet 对 Pod 的访问
- podStatuses：用于存储 Pod 的状态
- podStatusChannel：当其他组件调用 statusManager 更新 Pod 状态时，会调用这个 channel
- apiStatusVersions：维护最新的 Pod status 版本号，每次更新会加 1
- podDeletionSafety：删除 Pod 的接口

```go
func NewManager(kubeClient clientset.Interface, podManager kubepod.Manager, podDeletionSafety PodDeletionSafetyProvider) Manager {
	return &manager{
		kubeClient:        kubeClient,
		podManager:        podManager,
		podStatuses:       make(map[types.UID]versionedPodStatus),
		podStatusChannel:  make(chan podStatusSyncRequest, 1000), // Buffer up to 1000 statuses
		apiStatusVersions: make(map[kubetypes.MirrorPodUID]uint64),
		podDeletionSafety: podDeletionSafety,
	}
}
```

SyncHandler（pkg/kubelet/kubelet.go，195 行）是一个由 Kubelet 实现的接口，用于处理 Pod 的添加，更新，删除等事件。

```go
type SyncHandler interface {
	HandlePodAdditions(pods []*v1.Pod)
	HandlePodUpdates(pods []*v1.Pod)
	HandlePodRemoves(pods []*v1.Pod)
	HandlePodReconcile(pods []*v1.Pod)
	HandlePodSyncs(pods []*v1.Pod)
	HandlePodCleanups() error
}
```

初始化 PodConfig（pkg/kubelet/kubelet.go，275 行），这里面涉及到几个参数：
- Recorder：事件记录器（如 Pod 生命周期事件，各种错误事件）
- EventBroadcaster：事件分发器，分发给 watch 它的函数，用 channel 实现

```go
cfg := config.NewPodConfig(config.PodConfigNotificationIncremental, kubeDeps.Recorder)
```

### 手工调用 PodManager，创建一个假的静态 Pod

参考代码 pkg/kubelet/kubelet.go，623 行。

```go
// podManager is also responsible for keeping secretManager and configMapManager contents up-to-date.
mirrorPodClient := kubepod.NewBasicMirrorClient(klet.kubeClient, string(nodeName), nodeLister)
klet.podManager = kubepod.NewBasicPodManager(mirrorPodClient, secretManager, configMapManager)
```

运行程序调用 PodManager 创建 Pod。

```bash
cd kubernetes-1.22.15/mykubelet/
go run mytest/myclient/static_pod.go
```

查看创建的静态 Pod。删除可以使用 kubectl delete --force 命令强制删除。

```bash
> kubectl get pod
NAME                      READY   STATUS    RESTARTS   AGE
kube-mystatic-myjtthink   0/1     Pending   0          2s
```

### 监听 Pod 加入缓存

```bash
cd kubernetes-1.22.15/mykubelet/
go run mytest/myclient/pod_manager.go
```

浏览器输入 http://localhost:8080/pods ，可以看到指定节点当前的 Pod 列表。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230526161901.png)

创建一个新的 Pod。

```bash
kubectl apply -f yaml/nginx.yaml
```

刷新浏览器，可以看到 Pod 列表中出现了新的 Pod。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230526162036.png)

删除该 Pod。

```bash
kubectl delete -f yaml/nginx.yaml
```

刷新浏览器，Pod 已经从列表中消失。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230526161901.png)

### PodWorkers 

PodWorkers 的作用如下：
 - 1.每创建一个新的 Pod，都会为其创建一个专用的 PodWorkers。
 - 2.每个 PodWorkers 其实就一个协程，它会创建一个类型为 UpdatePodOptions（Pod 更新事件）的 channel。
 - 3.获得 Pod 更新事件后调用 PodWorkers 中的 syncPodFn（就是在 kubelet 里面有个 syncPod 函数）进行具体的同步工作。注意：SyncPod 就包含了将 Pod 的最新状态上报给 API Server，创建 Pod 的专属目录等等。

初始化代码在 pkg/kubelet/kubelet.go，656 行。

```go
klet.podWorkers = newPodWorkers(
    klet.syncPod,
    klet.syncTerminatingPod,
    klet.syncTerminatedPod,

    kubeDeps.Recorder,
    klet.workQueue,
    klet.resyncInterval,
    backOffPeriod,
    klet.podCache,
)
```

PodWorkers 中的 managePodLoop 方法的基本作用是监听 podUpdates 更新事件，从而触发 PodSyncFn（pkg/kubelet/pod_workers.go，936 行）。
managePodLoop 中有个关键的阻塞函数（pkg/kubelet/pod_workers.go，910 行），根据 uid 获取 Pod 的最新状态，这里面必须等待 podCache 有针对这个 Pod 的状态数据，才会继续往下执行。

```go
status, err = p.podCache.GetNewerThan(pod.UID, lastSyncTime)
```

手动调用 PodWorkers。

```bash
cd kubernetes-1.22.15/mykubelet/
go run mytest/myclient/pod_worker.go
```

创建一个新的 Pod。

```bash
kubectl apply -f yaml/nginx.yaml
```

查看 Pod 的 UID。

```bash
kubectl get pod nginx-kubelet -o yaml -o jsonpath='{.metadata.uid}'

# 返回结果
59367ef6-1bb2-4057-ba10-e71328a2c94e
```

打开浏览器输入 http://localhost:8080/setcache?id=59367ef6-1bb2-4057-ba10-e71328a2c94e 设置 PodCache，在控制台输出可以看到由于 GetNewerThan 根据 Pod uid 获取到 Pod 状态信息，因此执行了 PodSyncFn。

```bash
临时的syncpod函数
要处理的 Pod 名称是 nginx-kubelet
```
