[[TOC]]

## CSI 介绍

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230602104338.png)

通常情况下：CSI Driver = DaemonSet + Deployment(StatefuleSet)。

其中：
- 绿色部分：Identity、Node、Controller 是需要开发者自己实现的，被称为 Custom Components。
- 粉色部分：node-driver-registrar、external-attacher、external-provisioner 组件是 Kubernetes 团队开发和维护的，被称为 External Components，它们都是以 sidecar 的形式与 Custom Components 配合使用的。

Custom Components 本质是 3 个 gRPC Services：

- Identity Service：顾名思义，主要用于对外暴露这个插件本身的信息，比如驱动的名称、驱动的能力等。
- Controller Service：主要定义一些无需在宿主机上执行的操作，这也是与下文的 Node Service 最根本的区别。用于实现创建/删除 volume、attach/detach volume、volume 快照、volume 扩缩容等功能。以 CreateVolume 为例，k8s 通过调用该方法创建底层存储。比如底层使用了某云供应商的云硬盘服务，开发者在 CreateVolume 方法实现中应该调用云硬盘服务的创建/订购云硬盘的 API，调用 API 这个操作是不需要在特定宿主机上执行的。
- Node Service：定义了需要在宿主机上执行的操作，比如：mount、unmount。在前面的部署架构图中，Node Service 使用 Daemonset 的方式部署，也是为了确保 Node Service 会被运行在每个节点，以便执行诸如 mount 之类的指令。

ControllerPublishVolume（可选）: 卷创建好后，发布到某个节点（好比云盘购买后，需要挂到节点里，才能看到）
NodeStageVolume：将云硬盘格式化成对应文件系统，并且挂载到一个全局目录上（方便多 Pod 使用同一个卷，只需格式化一次）
NodePublishVolume（必须）：把宿主机目录（或全局目录）挂载到 Pod 里。   

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230602105434.png)

External Components 都是以 sidecar 的方式提供使用的。当开发完三个 Custom Components 之后，开发者需要根据存储的特点，选择合适的 sidecar 容器注入到 Pod 中。这里的 External Components 除了前面图中提到的 node-driver-registrar、external-attacher、external-provisioner 还有很多，可以参考官方文档，这里对常用的 sidecars 做一些简单介绍：
- livenessprobe：监视 CSI 驱动程序的运行状况，并将其报告给 Kubernetes。这使得 Kubernetes 能够自动检测驱动程序的问题，并重新启动 pod 来尝试修复问题。
- node-driver-registrar：可从 CSI driver 获取驱动程序信息（通过 NodeGetInfo 方法），并使用 kubelet 插件注册机制在该节点上的 kubelet 中对其进行注册。
- external-provisioner：组件对于块存储（如 ceph）非常关键。它监听 PersistentVolumeClaim 创建，调用 CSI 驱动的 CreateVolume 方法创建对应的底层存储（如 ceph image），一旦创建成功，provisioner 会创建一个 PersistentVolume 资源。当监听到 PersistentVolumeClaim 删除时，它会调用 CSI 的 DeleteVolume 方法删除底层存储，如果成功，则删除 PersistentVolume。
- external-attacher：用于监听 Kubernetes VolumeAttachment 对象并触发 CSI 的 Controller[Publish|Unpublish]Volume 操作。
- external-resizer：监听 PersistentVolumeClaim 资源修改，调用 CSI ControllerExpandVolume 方法，来调整 volume 的大小。

## 动态卷供应（Dynamic Volume Provisioning）执行过程

为了实现 Identity、Node、Controller 3个服务，需要清楚动态卷供应的执行过程。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230602104613.png)


## 部署 NFS 服务器

方式一：Docker 部署

```bash
mkdir -p /tmp/nfsdata
docker run -d --name nfsserver --network kind \
--privileged \
-v /tmp/nfsdata:/home/nfsdata \
-p 2049:2049 \
-e NFS_EXPORT_0="/home/nfsdata *(rw,fsid=1,async,insecure,no_root_squash)" \
erichough/nfs-server
```

方式二：手动安装

```bash
apt install nfs-kernel-server

mkdir -p /home/nfsdata
echo /home/nfsdata *(rw,async,insecure,no_root_squash,no_subtree_check) >> /etc/exports

systemctl start nfs-kernel-server
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

确认 Pod 运行成功。

```bash
root@instance-1:/# kubectl get pod
NAME                           READY   STATUS    RESTARTS   AGE
mycsi-nginx-67545f49f8-cwgn7   1/1     Running   0          12m
```

查看 mycsi 容器的日志，可以看到详细的执行过程：
- 1.external-provisioner 调用 ControllerService 的 CreateVolume 先临时挂载 172.18.0.1:/home/nfsdata 到 mycsi 容器的 /tmp 目录，然后在 /tmp 目录中创建 Pod PVC 专属的 pvc-bd42016c-a90d-4510-ad36-ea14fb979234 目录。然后卸载 /tmp 目录的挂载，这个操作的目的是为了在 NFS 服务器中创建出 Pod PVC 专属的目录，在 NodePublishVolume 阶段进行挂载。
- 2.external-provisioner 创建 PV，并将 PV 和 PVC 对象进行绑定。
- 3.VolumeController 的 AttachDetachController 控制循环发现 Volume 未被挂载到宿主机，需要 Attach 操作，于是创建 VolumeAttachment 对象。
- 4.external-attacher 监听到 VolumeAttachment 资源创建后，调用 Controller Service 的 ControllerPublishVolume 方法。注意这一步不是必须的，我们在 ControllerService 的 ControllerGetCapabilities 中设置了 csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME 从而跳过这一阶段。attach 的作用把块存储挂载到宿主机（好比一个磁盘），我们这里使用的是 NFS 可以直接提供文件系统，因此无需这一步骤。
- 5.kubelet 调用 NodeService 的 NodeStageVolume 方法，将 NFS 服务器上的 172.18.0.1:/home/nfsdata/pvc-bd42016c-a90d-4510-ad36-ea14fb979234 目录挂载到 Node 的 /var/lib/kubelet/pods/0b1b0b6d-0b1b-4b0b-9b0b-6d0b0b0b0b0b/volumes/kubernetes.io~csi/pvc-bd42016c-a90d-4510-ad36-ea14fb979234/mount 目录。

```bash
I0602 01:59:39.552667       1 options.go:11] 请求方法是: /csi.v1.Controller/CreateVolume
I0602 01:59:39.554511       1 options.go:15] 请求内容是: {"capacity_range":{"required_bytes":2147483648},"name":"pvc-bd42016c-a90d-4510-ad36-ea14fb979234","parameters":{"csi.storage.k8s.io/pv/name":"pvc-bd42016c-a90d-4510-ad36-ea14fb979234","csi.storage.k8s.io/pvc/name":"mycsi-pvc","csi.storage.k8s.io/pvc/namespace":"default","server":"172.18.0.1","share":"/home/nfsdata"},"volume_capabilities":[{"AccessType":{"Mount":{}},"access_mode":{"mode":1}}]}
I0602 01:59:39.555085       1 ControllerService.go:124] 调用 CreateVolume, 创建 volume
I0602 01:59:39.555123       1 ControllerService.go:126] PV 名称是pvc-bd42016c-a90d-4510-ad36-ea14fb979234
I0602 01:59:39.555152       1 ControllerService.go:127] 参数是:map[csi.storage.k8s.io/pv/name:pvc-bd42016c-a90d-4510-ad36-ea14fb979234 csi.storage.k8s.io/pvc/name:mycsi-pvc csi.storage.k8s.io/pvc/namespace:default server:172.18.0.1 share:/home/nfsdata]
W0602 01:59:39.659918       1 mount_helper_common.go:142] Warning: "/tmp/" is not a mountpoint, deleting
I0602 01:59:46.514964       1 options.go:11] 请求方法是: /csi.v1.Node/NodeGetCapabilities
I0602 01:59:46.514994       1 options.go:15] 请求内容是: {}
I0602 01:59:46.521891       1 options.go:11] 请求方法是: /csi.v1.Node/NodeGetCapabilities
I0602 01:59:46.522001       1 options.go:15] 请求内容是: {}
I0602 01:59:46.528876       1 options.go:11] 请求方法是: /csi.v1.Node/NodeGetCapabilities
I0602 01:59:46.529087       1 options.go:15] 请求内容是: {}
I0602 01:59:46.533038       1 options.go:11] 请求方法是: /csi.v1.Node/NodeGetCapabilities
I0602 01:59:46.533217       1 options.go:15] 请求内容是: {}
I0602 01:59:46.537257       1 options.go:11] 请求方法是: /csi.v1.Node/NodePublishVolume
I0602 01:59:46.537277       1 options.go:15] 请求内容是: {"target_path":"/var/lib/kubelet/pods/93526e52-bf18-4287-9842-f46fca064753/volumes/kubernetes.io~csi/pvc-bd42016c-a90d-4510-ad36-ea14fb979234/mount","volume_capability":{"AccessType":{"Mount":{}},"access_mode":{"mode":7}},"volume_context":{"storage.kubernetes.io/csiProvisionerIdentity":"1685671159668-8081-mycsi.jtthink.com"},"volume_id":"jtthink-volume-pvc-bd42016c-a90d-4510-ad36-ea14fb979234"}
I0602 01:59:46.540286       1 NodeService.go:29] 挂载参数： []
I0602 01:59:46.540404       1 NodeService.go:30] NodePublishVolume
I0602 01:59:46.540520       1 NodeService.go:35] 要挂载的目录是:/var/lib/kubelet/pods/93526e52-bf18-4287-9842-f46fca064753/volumes/kubernetes.io~csi/pvc-bd42016c-a90d-4510-ad36-ea14fb979234/mount
I0602 01:59:46.540760       1 NodeService.go:51] 要挂载的volume是：jtthink-volume-pvc-bd42016c-a90d-4510-ad36-ea14fb979234
I0602 01:59:46.540867       1 NodeService.go:53] 要挂载的pv是：jtthink-volume-pvc-bd42016c-a90d-4510-ad36-ea14fb979234
```

```bash
# 在 Pod 中查看挂载的目录
kubectl exec -it mycsi-nginx-67545f49f8-cwgn7 -- bash
root@mycsi-nginx-67545f49f8-cwgn7:/# ls /tmp/data
# 在目录中创建一个文件 
touch /tmp/data/podfile

# 在 Node 上查看 Pod PVC 目录
docker exec -it csi-demo-worker bash
root@csi-demo-worker:/# ls /var/lib/kubelet/pods/93526e52-bf18-4287-9842-f46fca064753/volumes/kubernetes.io~csi/pvc-bd42016c-a90d-4510-ad36-ea14fb979234/mount
podfile

# 在 NFS 服务器上查看 Pod PVC 的目录
root@instance-1:/# ls /home/nfsdata/pvc-bd42016c-a90d-4510-ad36-ea14fb979234
podfile
```


- CSINode 保存节点上安装的所有 CSI 驱动的状态信息。CSI驱动程序不需要直接创建 CSINode 对象。当 CSI 驱动程序通过 Kubelet 插件注册机制注册时，Kubelet 会在 CSINode 对象中添加 driver 的相关信息。
- external-attacher 监听 VolumeAttachment 对象，并调用 CSI 驱动程序来的 Controller Service 的 ControllerPublishVolume 方法来 attach volume。如果 CSI 驱动程序不支持 ControllerPublishVolume 调用，external-attacher 会直接将 VolumeAttachment 的 status.attached 设置为 true。

```yaml
root@instance-1:~# kubectl get csinodes csi-demo-worker -o yaml
apiVersion: storage.k8s.io/v1
kind: CSINode
metadata:
  annotations:
    storage.alpha.kubernetes.io/migrated-plugins: kubernetes.io/aws-ebs,kubernetes.io/azure-disk,kubernetes.io/azure-file,kubernetes.io/cinder,kubernetes.io/gce-pd,kubernetes.io/vsphere-volume
  creationTimestamp: "2023-05-31T12:47:13Z"
  name: csi-demo-worker
  ownerReferences:
  - apiVersion: v1
    kind: Node
    name: csi-demo-worker
    uid: f620516b-9f57-4892-830f-7c1e6a01c257
  resourceVersion: "217516"
  uid: dd5f7849-bd46-473f-b237-0f0df9b04348
spec:
  drivers:
  - name: mycsi.jtthink.com
    nodeID: csi-demo-worker
    topologyKeys: null
    
    
root@instance-1:~# kubectl get pvc mycsi-pvc -o yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"annotations":{},"name":"mycsi-pvc","namespace":"default"},"spec":{"accessModes":["ReadWriteOnce"],"resources":{"requests":{"storage":"2Gi"}},"storageClassName":"mycsi-sc"}}
    pv.kubernetes.io/bind-completed: "yes"
    pv.kubernetes.io/bound-by-controller: "yes"
    volume.beta.kubernetes.io/storage-provisioner: mycsi.jtthink.com
    volume.kubernetes.io/storage-provisioner: mycsi.jtthink.com
  creationTimestamp: "2023-06-02T01:59:25Z"
  finalizers:
    - kubernetes.io/pvc-protection
  name: mycsi-pvc
  namespace: default
  resourceVersion: "217575"
  uid: bd42016c-a90d-4510-ad36-ea14fb979234
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 2Gi
  storageClassName: mycsi-sc
  volumeMode: Filesystem
  volumeName: pvc-bd42016c-a90d-4510-ad36-ea14fb979234
status:
  accessModes:
    - ReadWriteOnce
  capacity:
    storage: 2Gi
  phase: Bound


root@instance-1:~# kubectl get pv pvc-bd42016c-a90d-4510-ad36-ea14fb979234 -o yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pv.kubernetes.io/provisioned-by: mycsi.jtthink.com
    volume.kubernetes.io/provisioner-deletion-secret-name: ""
    volume.kubernetes.io/provisioner-deletion-secret-namespace: ""
  creationTimestamp: "2023-06-02T01:59:39Z"
  finalizers:
    - kubernetes.io/pv-protection
  name: pvc-bd42016c-a90d-4510-ad36-ea14fb979234
  resourceVersion: "217572"
  uid: 23a6b11b-49bd-4616-970c-c2ea2051dd9c
spec:
  accessModes:
    - ReadWriteOnce
  capacity:
    storage: 2Gi
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: mycsi-pvc
    namespace: default
    resourceVersion: "217530"
    uid: bd42016c-a90d-4510-ad36-ea14fb979234
  csi:
    driver: mycsi.jtthink.com
    volumeAttributes:
      storage.kubernetes.io/csiProvisionerIdentity: 1685671159668-8081-mycsi.jtthink.com
    volumeHandle: jtthink-volume-pvc-bd42016c-a90d-4510-ad36-ea14fb979234
  persistentVolumeReclaimPolicy: Delete
  storageClassName: mycsi-sc
  volumeMode: Filesystem
status:
  phase: Bound


root@instance-1:~# kubectl get volumeattachments csi-f71c3731eac9e156d0923ea2b6ce8a9b16f1f5417afac82c7854a56818acd9cc -o yaml
apiVersion: storage.k8s.io/v1
kind: VolumeAttachment
metadata:
  creationTimestamp: "2023-06-02T01:59:42Z"
  name: csi-f71c3731eac9e156d0923ea2b6ce8a9b16f1f5417afac82c7854a56818acd9cc
  resourceVersion: "217583"
  uid: 70bf204f-9a4d-4a7a-8bb0-3890ff62c775
spec:
  attacher: mycsi.jtthink.com
  nodeName: csi-demo-worker
  source:
    persistentVolumeName: pvc-bd42016c-a90d-4510-ad36-ea14fb979234
status:
  attached: true
```


## 参考资料
- [Developing CSI Driver for Kubernetes](https://kubernetes-csi.github.io/docs/developing.html)
- [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec/blob/master/spec.md)
- [k8s CSI插件开发基础入门篇](https://www.jtthink.com/course/208)
- [CSI 驱动开发指南](https://cloudnative.to/blog/develop-a-csi-driver/#csi-%E7%BB%84%E6%88%90)
- [开发自己的Kubernetes CSI存储插件](https://blog.dianduidian.com/post/%E5%BC%80%E5%8F%91%E8%87%AA%E5%B7%B1%E7%9A%84csi%E5%AD%98%E5%82%A8%E6%8F%92%E4%BB%B6/)
- [官方示例：kubernetes-csi/csi-driver-host-path](https://github.com/kubernetes-csi/csi-driver-host-path/tree/master)