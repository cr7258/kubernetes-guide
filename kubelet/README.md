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
