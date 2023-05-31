[TOC]

## 部署 NFS 服务器

```bash
mkdir -p /tmp/nfsdata
docker run -d --name nfsserver --network kind \
--privileged \
-v /tmp/nfsdata:/home/nfsdata \
-p 2049:2049 \
-e NFS_EXPORT_0="/home/nfsdata *(rw,fsid=1,async,insecure,no_root_squash)" \
erichough/nfs-server
```

验证是否可以挂载 NFS。

```bash
# 查看容器 IP
docker inspect nfsserver | grep IPAddress
mkdir /tmp/testdata
# 同时需要替换代码和配置文件中的 IP 地址为你自己 NFS 服务器的 IP 地址
mount -t nfs 172.18.0.1:/home/nfsdata /tmp/testdata
```

## 创建 Kubernetes 集群

```bash
kind create cluster --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: csi-demo
nodes:
  - role: control-plane
  - role: worker
EOF
```

## 编译代码

```bash
sh boot.sh
```

将编译好的 mycsi 文件拷贝到 csi-demo-worker 节点上。

```bash
docker exec csi-demo-worker mkdir -p /home/csi
docker cp bin/mycsi csi-demo-worker:/home/csi
```

## 部署 CSI 容器

```bash
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/driver.yaml
```

确认容器正常运行：

```bash
root@instance-1:~/kubernetes-guide/csi# kubectl get pod -n mycsi
NAME                    READY   STATUS    RESTARTS   AGE
mycsi-99d58db6d-wh7sw   5/5     Running   0          6s
```

## 部署 Pod 使用 PVC

```bash
kubectl apply -f deploy/testcsi.yaml
```


## 参考资料
- [k8s CSI插件开发基础入门篇](https://www.jtthink.com/course/208)
- [CSI 驱动开发指南](https://cloudnative.to/blog/develop-a-csi-driver/#csi-%E7%BB%84%E6%88%90)