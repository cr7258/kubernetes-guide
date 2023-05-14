## 准备 Etcd

```bash
ETCD_VER=v3.4.26

# choose either URL
GOOGLE_URL=https://storage.googleapis.com/etcd
GITHUB_URL=https://github.com/etcd-io/etcd/releases/download
DOWNLOAD_URL=${GOOGLE_URL}

rm -f /tmp/etcd-${ETCD_VER}-darwin-amd64.zip
rm -rf /tmp/etcd-download-test && mkdir -p /tmp/etcd-download-test

curl -L ${DOWNLOAD_URL}/${ETCD_VER}/etcd-${ETCD_VER}-darwin-amd64.zip -o /tmp/etcd-${ETCD_VER}-darwin-amd64.zip
unzip /tmp/etcd-${ETCD_VER}-darwin-amd64.zip -d /tmp

# 将 etcd 相关的文件移动到自己的目录下
mv /tmp/etcd-${ETCD_VER}-darwin-amd64/etc* ~/software/bin/

# 启动的时候只需要执行以下命令即可
etcd
```

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230514131259.png)

## 生成 API Server 所需的证书

进入 kubernetes-1.24.12/certs 目录，然后执行以下命令。

生成一个 CA (证书颁发机构) 证书和私钥，用于签名和验证 Kubernetes API Server 和 Service Account 证书。

```bash
openssl genrsa -out ca.key 2048
openssl req -new -key ca.key -subj "/CN=Kubernetes CA" -out ca.csr
openssl x509 -req -in ca.csr -signkey ca.key -out ca.crt
```
生成 Kubernetes API Server 的证书和私钥。

```bash
openssl genrsa -out apiserver.key 2048
openssl req -new -key apiserver.key -subj "/CN=kube-apiserver" -out apiserver.csr
openssl x509 -req -in apiserver.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out apiserver.crt -days 365
```
生成 Service Account 的证书和私钥。

```bash
openssl genrsa -out sa.key 2048
openssl req -new -key sa.key -subj "/CN=service-account" -out sa.csr
openssl x509 -req -in sa.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out sa.crt -days 365
```
## 启动 API Server

下载 Kubernetes 1.24.12 版本：https://github.com/kubernetes/kubernetes/tree/v1.24.12

```bash
cd kubernetes-1.24.12
./boot.sh
```
boot.sh 内容如下：

```bash
go run -mod=mod cmd/kube-apiserver/apiserver.go \
--etcd-servers=http://127.0.0.1:2379 \
--service-account-issuer=https://kubernetes.default.svc.cluster.local \
--authorization-mode=Node,RBAC \
--service-account-key-file=./certs/sa.crt \
--service-account-signing-key-file=./certs/sa.key \
--service-cluster-ip-range=10.96.0.0/12 \
--tls-cert-file=./certs/apiserver.crt \
--tls-private-key-file=./certs/apiserver.key \
--feature-gates=TTLAfterFinished=true,EphemeralContainers=true
```
浏览器输入 https://localhost:6443 访问 API Server，此时 API Server 会响应 403 Forbidden 错误，这是正常的，因为用户没有相应的权限。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230514134503.png)

## 不使用 RBAC 鉴权、拦截用户权限请求

接下来我们跳过 API Server 的 RBAC 鉴权，使用自定义的准入控制器来拦截用户的权限请求。

```bash
# 修改 --authorization-mode=Node,RBAC 为
--authorization-mode=Node,Webhook

# 添加
--runtime-config=authorization.k8s.io/v1beta1=true
--authorization-webhook-config-file=../webhook/config
```

Webhook Server 始终允许所有请求，代码：webhook/main.go
参考：https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/webhook/#request-payloads

启动 Webhook Server

```bash
cd webhook
go run main.go
```

重新执行 boot.sh 启动 API Server，再次访问 API Server，这次就可以正常访问了。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230514141145.png)


## client-go 请求本地 Apiserver，创建 Default SA

default Service Account 的作用：
- 如果你没有指定，那么这个账户将被自动创建。这个账户是由 controller-manager 创建的。
- 它具有访问该 Namespace 内的大多数资源的权限，包括查看和创建 Pod、Service、Deployment 等资源。

源码位置：pkg/controller/serviceaccount/serviceaccounts_controller.go: 185 行

执行 client-go 请求本地 Apiserver，创建 Default SA。
```bash
cd webhook
go run test.go

# 输出我们通过 Clientset 创建的 Pod 名称
Pod: nginxpod
```

## 查看 K8S 存在 Etcd 中的数据结构

```bash
etcdctl get / --prefix --keys-only

# registry 开头的就是 K8S 相关的数据
/registry/apiregistration.k8s.io/apiservices/v1.

/registry/apiregistration.k8s.io/apiservices/v1.admissionregistration.k8s.io

/registry/apiregistration.k8s.io/apiservices/v1.apiextensions.k8s.io

/registry/apiregistration.k8s.io/apiservices/v1.apps

/registry/apiregistration.k8s.io/apiservices/v1.authentication.k8s.io

/registry/apiregistration.k8s.io/apiservices/v1.authorization.k8s.io

/registry/apiregistration.k8s.io/apiservices/v1.autoscaling

/registry/apiregistration.k8s.io/apiservices/v1.batch

/registry/apiregistration.k8s.io/apiservices/v1.certificates.k8s.io

/registry/apiregistration.k8s.io/apiservices/v1.coordination.k8s.io

/registry/apiregistration.k8s.io/apiservices/v1.discovery.k8s.io

/registry/apiregistration.k8s.io/apiservices/v1.events.k8s.io
......
```

获取 Pods 列表。

```bash
etcdctl get /registry/pods --prefix --keys-only

# 输出
/registry/pods/default/nginxpod
```

获取指定 Pod 的内容。

```bash
etcdctl get /registry/pods/default/nginxpod -w json | jq

# 输出
{
  "header": {
    "cluster_id": 14841639068965180000,
    "member_id": 10276657743932975000,
    "revision": 538,
    "raft_term": 2
  },
  "kvs": [
    {
      "key": "L3JlZ2lzdHJ5L3BvZHMvZGVmYXVsdC9uZ2lueHBvZA==",
      "create_revision": 474,
      "mod_revision": 474,
      "version": 1,
      "value": "azhzAAoJCgJ2MRIDUG9kEu8HCs4DCghuZ2lueHBvZBIAGgdkZWZhdWx0IgAqJDE5OGIxZTRlLTIwZWEtNDU3Ny1hNDg3LTA3NDEzYWExMWVjNTIAOABCCAjJ+oGjBhAAegCKAf0CCgR0ZXN0EgZVcGRhdGUaAnYxIggIyfqBowYQADIIRmllbGRzVjE60gIKzwJ7ImY6c3BlYyI6eyJmOmNvbnRhaW5lcnMiOnsiazp7XCJuYW1lXCI6XCJuZ2lueFwifSI6eyIuIjp7fSwiZjppbWFnZSI6e30sImY6aW1hZ2VQdWxsUG9saWN5Ijp7fSwiZjpuYW1lIjp7fSwiZjpyZXNvdXJjZXMiOnt9LCJmOnRlcm1pbmF0aW9uTWVzc2FnZVBhdGgiOnt9LCJmOnRlcm1pbmF0aW9uTWVzc2FnZVBvbGljeSI6e319fSwiZjpkbnNQb2xpY3kiOnt9LCJmOmVuYWJsZVNlcnZpY2VMaW5rcyI6e30sImY6cmVzdGFydFBvbGljeSI6e30sImY6c2NoZWR1bGVyTmFtZSI6e30sImY6c2VjdXJpdHlDb250ZXh0Ijp7fSwiZjp0ZXJtaW5hdGlvbkdyYWNlUGVyaW9kU2Vjb25kcyI6e319fUIAEvoDCoQBChVrdWJlLWFwaS1hY2Nlc3MteGdiZjgSa9IBaAoOIgwKABCXHBoFdG9rZW4KKBomChIKEGt1YmUtcm9vdC1jYS5jcnQSEAoGY2EuY3J0EgZjYS5jcnQKKRInCiUKCW5hbWVzcGFjZRIYCgJ2MRISbWV0YWRhdGEubmFtZXNwYWNlEKQDEo4BCgVuZ2lueBIFbmdpbngqAEIASkwKFWt1YmUtYXBpLWFjY2Vzcy14Z2JmOBABGi0vdmFyL3J1bi9zZWNyZXRzL2t1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQiADIAahQvZGV2L3Rlcm1pbmF0aW9uLWxvZ3IGQWx3YXlzgAEAiAEAkAEAogEERmlsZRoGQWx3YXlzIB4yDENsdXN0ZXJGaXJzdEIHZGVmYXVsdEoHZGVmYXVsdFIAWABgAGgAcgCCAQCKAQCaARFkZWZhdWx0LXNjaGVkdWxlcrIBNgocbm9kZS5rdWJlcm5ldGVzLmlvL25vdC1yZWFkeRIGRXhpc3RzGgAiCU5vRXhlY3V0ZSisArIBOAoebm9kZS5rdWJlcm5ldGVzLmlvL3VucmVhY2hhYmxlEgZFeGlzdHMaACIJTm9FeGVjdXRlKKwCwgEAyAEA8AEB+gEUUHJlZW1wdExvd2VyUHJpb3JpdHkaHwoHUGVuZGluZxoAIgAqADIASgpCZXN0RWZmb3J0WgAaACIA"
    }
  ],
  "count": 1
}
```

## 简化代码启动服务

```bash
# mac 上需要执行以下步骤
mkdir -p /var/run/kubernetes
sudo chown -R <用户名> /var/run/kubernetes
```

使用简化的参数启动 API Server。

```bash
cd kubernetes-1.24.12/001/rest
go run test.go

# 输出以下内容
# 经过 completedOptions.Validate() 校验，会提示我们缺少哪些参数
[--etcd-servers must be specified service-account-issuer is a required flag --service-account-signing-key-file and --service-account-issuer are required flags]
```

缺少的参数如下：
- etcd-servers: Etcd 的连接信息
- service-account-issuer: ServiceAccount Token中 的签发身份，即 Token payload 中的 iss 字段。默认 https://kubernetes.default.svc
- api-audiences: 合法的请求 Token 身份，用于 apiserver 服务端认证请求 Token 是否合法。
- service-account-signing-key-file: Token签名私钥文件路径

填充以下参数。

```go
completedOptions.Etcd.StorageConfig.Transport.ServerList = EtcdServers
completedOptions.Authentication.ServiceAccounts.Issuers = Issuers
completedOptions.Authentication.APIAudiences = Issuers
completedOptions.Authentication.ServiceAccounts.KeyFiles = []string{"../../certs/sa.crt"}
completedOptions.ServiceAccountSigningKeyFile = "../../certs/sa.key"

sk, err := keyutil.PrivateKeyFromFile(completedOptions.ServiceAccountSigningKeyFile)
completedOptions.ServiceAccountIssuer, err = serviceaccount.JWTTokenGenerator(completedOptions.Authentication.ServiceAccounts.Issuers[0], sk)
```

然后重新启动 API Server 就没有报错了。

```bash
# 输出结果
W0514 20:03:08.722304   97857 services.go:37] No CIDR for service cluster IPs specified. Default value which was 10.0.0.0/24 is deprecated and will be removed in future releases. Please specify it using --service-cluster-ip-range on kube-apiserver.
I0514 20:03:08.728561   97857 server.go:558] external host was not specified, using 192.168.2.150
W0514 20:03:08.728579   97857 authentication.go:526] AnonymousAuth is not allowed with the AlwaysAllow authorizer. Resetting AnonymousAuth to false. You should use a different authorizer
```

加入 Run 相关代码。

```go
// return Run(completedOptions, genericapiserver.SetupSignalHandler())

ch := genericapiserver.SetupSignalHandler()
server, err := app.CreateServerChain(completedOptions, ch)
if err != nil {
    log.Fatalln(err)
}

prepared, err := server.PrepareRun()
if err != nil {
    log.Fatalln(err)
}
prepared.Run(ch)
```

浏览器输入 https://localhost:6443 访问 API Server，此时 API Server 会响应 401 Unauthorized 错误，接下来我们会在代码中去除权限认证。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230514201119.png)

## 去除权限认证

在 kubernetes-1.24.12/001/rest/test.go 中添加以下内容可以去除权限认证。

```go
completedOptions.Authentication.Anonymous.Allow = true
completedOptions.Authorization.Modes = []string{authzmodes.ModeAlwaysAllow}
```