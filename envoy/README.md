* [Istio 微服务实战进阶之 Envoy 篇](#istio-微服务实战进阶之-envoy-篇)
    * [准备 Kubernetes 集群](#准备-kubernetes-集群)
    * [第一章 Istio 复习](#第一章-istio-复习)
        * [部署测试应用](#部署测试应用)
        * [检查服务网格](#检查服务网格)
    * [第二章 Envoy 学习之动态配置和控制面](#第二章-envoy-学习之动态配置和控制面)
        * [使用静态配置启动 Envoy，负载均衡代理服务](#使用静态配置启动-envoy负载均衡代理服务)
        * [动态配置入门，xDS 协议文件模式](#动态配置入门xds-协议文件模式)
        * [控制面动态配置](#控制面动态配置)
            * [启动服务和测试](#启动服务和测试)
            * [Envoy 使用动态配置连接控制面获取配置](#envoy-使用动态配置连接控制面获取配置)
            * [动态更新配置](#动态更新配置)
    * [第三章 Envoy 监听器和过滤器](#第三章-envoy-监听器和过滤器)
        * [Listener filters](#listener-filters)
        * [Network filters](#network-filters)
        * [HTTP filters](#http-filters)
    * [第四章 Envoy 流量拦截](#第四章-envoy-流量拦截)

# Istio 微服务实战进阶之 Envoy 篇 

本教程基于 Istio 1.18，Kubernetes 1.27 版本。

## 准备 Kubernetes 集群

```bash
kind create cluster --name istio-demo
```

## 第一章 Istio 复习

根据操作系统的版本下载对应的 Istio 压缩包：https://github.com/istio/istio/releases/tag/1.18.0 ，下载完成后解压可以在 [istio-1.18.0/manifests/profiles](./istio-1.18.0/manifests/profiles) 目录下看到 Istio 各种 profile 的配置文件。各个 profile 安装的组件如下图所示：

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230624185548.png)

profile 的配置文件也可以用 istioctl profile dump 命令进行生成，例如：

```bash
# 生成 default profile 的配置
istioctl profile dump

# 生成 demo profile 的配置
istioctl profile dump demo

# 只生成 ingressGateway 的配置
istioctl profile dump --config-path components.ingressGateways
```

执行以下命令按照 minimal profile 安装 Istio。

```bash
istioctl install --set profile=minimal
```

接下来我们选择自行安装 Istio IngressGateway。

```bash
istioctl install  -f  install/ingressgateway.yaml
```

要卸载 Istio 的话可以执行以下命令：

```bash
istioctl x uninstall --purge
```

### 部署测试应用

```bash
kubectl create ns myweb
# 注入 sidecar
kubectl label ns myweb istio-injection=enabled 
kubectl apply -f demo/prod.yaml
```

### 检查服务网格

查看 Pod 或者 Service 关联的 Istio 的配置。

```bash
istioctl x describe pod prodapi-7cc9555749-x6cct -n myweb

# 返回结果
Pod: prodapi-7cc9555749-x6cct
   Pod Revision: default
   Pod Ports: 8080 (prod), 15090 (istio-proxy)
--------------------
Service: prodsvc
   Port: http 8080/HTTP targets pod port 8080
--------------------
Effective PeerAuthentication:
   Workload mTLS mode: PERMISSIVE


Exposed on Ingress Gateway http://172.19.0.2
VirtualService: prodvs
   Match: /*
```

使用 istioctl proxy-status 命令可以看到网络与 Envoy 配置的同步状态。

- SYNCED 意思是 Envoy 知晓了 Istiod 已经将最新的配置发送给了它。
- NOT SENT 意思是 Istiod 没有发送任何信息给 Envoy。这通常是因为 Istiod 没什么可发送的。
- STALE 意思是 Istiod 已经发送了一个更新到 Envoy，但还没有收到应答。这通常意味着 Envoy 和 Istiod 之间存在网络问题，或者 Istio 自身的 bug。

信息来源于 xDS, xDS 是控制平面和数据平面(envoy)的通信接口，例如：
- CDS 集群发现服务
- LDS 监听器发现服务
- EDS 端点发现服务
- RDS 路由发现

```bash
> istioctl proxy-status
NAME                                            CLUSTER        CDS        LDS        EDS        RDS        ECDS         ISTIOD                      VERSION
ingressgateway-69db4f8cc-xfwxf.istio-system     Kubernetes     SYNCED     SYNCED     SYNCED     SYNCED     NOT SENT     istiod-5855798659-q286n     1.18.0
prodapi-7cc9555749-x6cct.myweb                  Kubernetes     SYNCED     SYNCED     SYNCED     SYNCED     NOT SENT     istiod-5855798659-q286n     1.18.0
```

使用 istioctl proxy-config 命令可以查看 Envoy 的配置。

```bash
istioctl proxy-config listener prodapi-7cc9555749-x6cct -n myweb

# 这里查看了 Listener 相关的配置
ADDRESS       PORT  MATCH                                                                                           DESTINATION
10.96.0.10    53    ALL                                                                                             Cluster: outbound|53||kube-dns.kube-system.svc.cluster.local
0.0.0.0       80    Trans: raw_buffer; App: http/1.1,h2c                                                            Route: 80
0.0.0.0       80    ALL                                                                                             PassthroughCluster
10.96.0.1     443   ALL                                                                                             Cluster: outbound|443||kubernetes.default.svc.cluster.local
10.96.109.245 443   ALL                                                                                             Cluster: outbound|443||ingressgateway.istio-system.svc.cluster.local
10.96.178.15  443   ALL                                                                                             Cluster: outbound|443||istiod.istio-system.svc.cluster.local
0.0.0.0       8080  Trans: raw_buffer; App: http/1.1,h2c                                                            Route: 8080
0.0.0.0       8080  ALL                                                                                             PassthroughCluster
10.96.0.10    9153  Trans: raw_buffer; App: http/1.1,h2c                                                            Route: kube-dns.kube-system.svc.cluster.local:9153
10.96.0.10    9153  ALL                                                                                             Cluster: outbound|9153||kube-dns.kube-system.svc.cluster.local
0.0.0.0       15001 ALL                                                                                             PassthroughCluster
0.0.0.0       15001 Addr: *:15001                                                                                   Non-HTTP/Non-TCP
0.0.0.0       15006 Addr: *:15006                                                                                   Non-HTTP/Non-TCP
0.0.0.0       15006 Trans: tls; App: istio-http/1.0,istio-http/1.1,istio-h2; Addr: 0.0.0.0/0                        InboundPassthroughClusterIpv4
0.0.0.0       15006 Trans: raw_buffer; App: http/1.1,h2c; Addr: 0.0.0.0/0                                           InboundPassthroughClusterIpv4
0.0.0.0       15006 Trans: tls; App: TCP TLS; Addr: 0.0.0.0/0                                                       InboundPassthroughClusterIpv4
0.0.0.0       15006 Trans: raw_buffer; Addr: 0.0.0.0/0                                                              InboundPassthroughClusterIpv4
0.0.0.0       15006 Trans: tls; Addr: 0.0.0.0/0                                                                     InboundPassthroughClusterIpv4
0.0.0.0       15006 Trans: tls; App: istio,istio-peer-exchange,istio-http/1.0,istio-http/1.1,istio-h2; Addr: *:8080 Cluster: inbound|8080||
0.0.0.0       15006 Trans: raw_buffer; Addr: *:8080                                                                 Cluster: inbound|8080||
0.0.0.0       15010 Trans: raw_buffer; App: http/1.1,h2c                                                            Route: 15010
0.0.0.0       15010 ALL                                                                                             PassthroughCluster
10.96.178.15  15012 ALL                                                                                             Cluster: outbound|15012||istiod.istio-system.svc.cluster.local
0.0.0.0       15014 Trans: raw_buffer; App: http/1.1,h2c                                                            Route: 15014
0.0.0.0       15014 ALL                                                                                             PassthroughCluster
0.0.0.0       15021 ALL                                                                                             Inline Route: /healthz/ready*
10.96.109.245 15021 Trans: raw_buffer; App: http/1.1,h2c                                                            Route: ingressgateway.istio-system.svc.cluster.local:15021
10.96.109.245 15021 ALL                                                                                             Cluster: outbound|15021||ingressgateway.istio-system.svc.cluster.local
0.0.0.0       15090 ALL                                                                                             Inline Route: /stats/prometheus*
```

## 第二章 Envoy 学习之动态配置和控制面

提前准备一个 Centos 服务器，并安装 Docker。

### 使用静态配置启动 Envoy，负载均衡代理服务

启动两个后端服务：

```bash
docker run --name ngxv1 -d nginx:1.18
docker exec -it ngxv1 sh -c "echo v1 > /usr/share/nginx/html/index.html"

docker run --name ngxv2 -d docker.io/nginx:1.18
docker exec -it ngxv2 sh -c "echo v2 > /usr/share/nginx/html/index.html"
```

修改 [Envoy 配置文件](./envoy/config/static/envoy.yaml) 中 cluster 配置的后端地址，然后启动 Envoy。

```bash
docker run --name=envoy -d \
-p 8080:8080 \
-p 9901:9901 \
-v ./envoy/config/static/envoy.yaml:/etc/envoy/envoy.yaml \
envoyproxy/envoy-alpine:v1.21.0
```

客户端请求 Envoy。

```bash
for i in {1..10}; do curl localhost:8080; done

# 返回结果，我们设置了 v2 和 v1 的比例的 6:4
v2
v1
v2
v1
v1
v2
v2
v2
v2
v2
```

浏览器输入 http://localhost:9901 可以访问 Envoy 的配置界面。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230624194110.png)

### 动态配置入门，xDS 协议文件模式

动态配置有两种形式：
- 1.基于文件系统，当文件被更改时，Envoy 自动更新更新。
- 2.基于控制面(例如 Gloo, Istio)

Envoy 通过监控指定路径下的符合格式的文件、 gRPC 流调用或轮询 rest api 来获取资源配置。

启动 Envoy，其中 lds.yaml 用于 listeners 配置，cds.yaml 用于 clusters 配置。

```bash
docker run --name=envoy -d \
  -p 8080:8080 \
  -p 9901:9901 \
  -v ./envoy//config/dynamic/filesystem:/etc/envoy \
  envoyproxy/envoy-alpine:v1.21.0
```

客户端请求 Envoy。

```bash
curl localhost:8080

# 返回结果
v1
```

### 控制面动态配置

#### 启动服务和测试

启动实现 xDS 协议的控制面：

```bash
go run controlplane/cmd/main.go
```

启动客户端通过 gRPC 访问控制面获取 Cluster 配置。

```bash
go run controlplane/cmd/client.go

# 返回结果
name:"jtthink_cluster" type:LOGICAL_DNS connect_timeout:{seconds:5} load_assignment:{cluster_name:"jtthink_cluster" endpoints:{lb_endpoints:{endpoint:{address:{socket_address:{address:"172.18.0.2" port_value:80}}}}}} dns_lookup_family:V4_ONLY
```

#### Envoy 使用动态配置连接控制面获取配置

启动实现 xDS 协议的控制面：

```bash
go run controlplane/cmd/main.go
```

启动 Envoy。

```bash
docker run --name=envoy -d \
  -p 8080:8080 \
  -p 9901:9901 \
  -v ./envoy//config/dynamic/controlplane:/etc/envoy \
  envoyproxy/envoy-alpine:v1.21.0
```

客户端请求 Envoy。

```bash
curl localhost:8080

# 返回结果
v1
```

#### 动态更新配置

控制面动态更新的主要代码如下，通过修改 SetSnapshot 修改缓存快照来更新配置，注意每次更新需要修改 version 才会生效。

```go
utils.UpstreamHost = "10.88.0.3"
ss := utils.GenerateSnapshot("v2")
err := configCache.SetSnapshot(c, nodeID, ss)
```

客户端请求 Envoy，刚开始流量将会发到 nginx v1。

```bash
curl localhost:8080

# 返回结果
v2
```

触发 Controlplane 修改配置，将 Cluster 的 upstream 从 nginx v1 改为 nginx v2。

```bash
curl 192.168.2.150:18000/test
```

客户端请求 Envoy，此时流量将会发到 nginx v2。

```bash
curl localhost:8080

# 返回结果
v2
```
## 第三章 Envoy 监听器和过滤器

Envoy 有 Listener filters, Network filters, HTTP filters 等多种过滤器，具体参考：https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/filter/filter

### Listener filters

Listener filters 用于管理连接的元数据，例如可以用来获取 HTTP/1.x, HTTP/2, TLS 的连接情况。

以下配置设置一个 HTTP Inspector listener filter 来获取 HTTP/1.x, HTTP/2 的连接情况。

```yaml
listener_filters:
  - name: "envoy.filters.listener.http_inspector"
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.listener.http_inspector.v3.HttpInspector
```

启动 Envoy。

```bash
docker run --name=envoy -d \
  -p 8080:8080 \
  -p 9901:9901 \
  -v ./envoy//config/filters/listener_filters:/etc/envoy \
  envoyproxy/envoy-alpine:v1.21.0
```

客户端请求 Envoy。

```bash
curl localhost:8080

# 返回结果
v1
```

访问 http://localhost:9901/stats/prometheus 可以看到 Envoy 输出的统计信息。

```bash
......
# TYPE envoy_http1_response_flood counter
envoy_http1_response_flood{} 0

# TYPE envoy_http_inspector_http10_found counter
envoy_http_inspector_http10_found{} 0

# TYPE envoy_http_inspector_http11_found counter
envoy_http_inspector_http11_found{} 3

# TYPE envoy_http_inspector_http2_found counter
envoy_http_inspector_http2_found{} 0

# TYPE envoy_http_inspector_http_not_found counter
envoy_http_inspector_http_not_found{} 0

# TYPE envoy_http_inspector_read_error counter
envoy_http_inspector_read_error{} 0
......
```

### Network filters

Network filters 处理 L3/L4 TCP 相关的流量。

以下配置使用 HTTP connection manager Network filters 为 HTTP Headers 中添加 name 和 age 两个字段，并且只允许 Host: abc.com 的请求访问。

```yaml
name: envoy.filters.network.http_connection_manager
typed_config:
"@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
stat_prefix: ingress_http
codec_type: AUTO
route_config:
  name: jtroute
  response_headers_to_add:
    - header:
        key: myname
        value: chengzw
    - header:
        key: age
        value: "18"
  virtual_hosts:
    - name: myhost
      domains: ["*"]
      routes:
        - match: {prefix: "/"}
          route: {cluster: jtthink_cluster_config}
```

启动 Envoy。

```bash
docker run --name=envoy -d \
  -p 8080:8080 \
  -p 9901:9901 \
  -v ./envoy//config/filters/network_filters:/etc/envoy \
  envoyproxy/envoy-alpine:v1.21.0
```

客户端请求 Envoy。

```bash
curl localhost:8080 -H "Host: abc.com" -i

# 返回结果
# 响应头
HTTP/1.1 200 OK
server: envoy
date: Wed, 28 Jun 2023 14:27:20 GMT
content-type: text/html
content-length: 3
last-modified: Sat, 24 Jun 2023 11:29:48 GMT
etag: "6496d3ac-3"
accept-ranges: bytes
x-envoy-upstream-service-time: 0
myname: chengzw
age: 18

# 响应体
v1
```

我们还可以使用 HTTP connection manager 来对 HTTP 的请求路径进行重写。将请求路径 /abc/xxx 重写为 /xxx.html。

```yaml
routes:
- match: {prefix: "/abc"}
  route:
    cluster: jtthink_cluster_config
    regex_rewrite:
      pattern:
        google_re2:
          max_program_size: 100
        regex: "^/abc/(.*?)$"
      substitution: "/\\1.html"
```

启动 Envoy。

```bash
docker run --name=envoy -d \
  -p 8080:8080 \
  -p 9901:9901 \
  -v ./envoy//config/filters/network_filters_rewrite:/etc/envoy \
  envoyproxy/envoy-alpine:v1.21.0
```

客户端请求 Envoy，路径 /abc/index 会被重写为 /index.html，这样就可以成功访问到 nginx 的 index.html 页面了。 

```bash
curl localhost:8080/abc/index -H "Host: abc.com" -i

# 返回结果
HTTP/1.1 200 OK
server: envoy
date: Wed, 28 Jun 2023 14:35:08 GMT
content-type: text/html
content-length: 3
last-modified: Sat, 24 Jun 2023 11:29:48 GMT
etag: "6496d3ac-3"
accept-ranges: bytes
x-envoy-upstream-service-time: 0
myname: chengzw
age: 18

v1
```

### HTTP filters

HTTP filters 专门针对 HTTP 的请求和响应进行处理，例如 CORS 跨域，插入 Lua 脚本等等。

以下是一个使用 Lua HTTP filter 往 TTP 响应头里插入 location 字段的配置。

```yaml
http_filters:
  - name: myheader.lua
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
      inline_code: |
        function envoy_on_response(response_handle)
            response_handle:headers():add("location", "China")
        end
```

启动 Envoy。

```bash
docker run --name=envoy -d \
  -p 8080:8080 \
  -p 9901:9901 \
  -v ./envoy//config/filters/http_filters:/etc/envoy \
  envoyproxy/envoy-alpine:v1.21.0
```

客户端请求 Envoy。

```bash
curl localhost:8080 -H "Host: abc.com" -i

# 返回结果
HTTP/1.1 200 OK
server: envoy
date: Wed, 28 Jun 2023 14:46:03 GMT
content-type: text/html
content-length: 3
last-modified: Sat, 24 Jun 2023 11:29:48 GMT
etag: "6496d3ac-3"
accept-ranges: bytes
x-envoy-upstream-service-time: 0
myname: chengzw
age: 18
location: China # Lua 脚本插入的响应头

v1
```

## 第四章 Envoy 流量拦截

