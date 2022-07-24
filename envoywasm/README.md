# 安装 Tinygo

```bash
# MacOS
brew tap tinygo-org/tools
brew install tinygo
```

参考链接：https://tinygo.org/getting-started/install/macos/

# 编译 wasm 文件

```bash
./build
```

每次重新编译后，需要重启 envoy 容器使配置生效。

```bash
docker restart envoy
```

# 启动容器

```bash

# 创建一个 bridge 类型的网络，nginx 和 envoy 容器共享相同的网络，envoy 可以通过 ngx 名称来 DNS 解析到 nginx 容器的 IP 地址
docker network create envoy 

# 后端服务
docker run --name ngx --net=envoy -d -p 8001:80 nginx:1.18-alpine

# envoy 代理，-v 是宿主机配置文件的路径，根据实际情况修改
docker run --name=envoy --net=envoy -d \
  -p 8080:8080 \
  -v /Users/chengzhiwei/Code/github/kubernetes/kubernetes-guide/envoywasm/envoy/config/envoy.yaml:/etc/envoy/envoy.yaml \
  -v /Users/chengzhiwei/Code/github/kubernetes/kubernetes-guide/envoywasm/envoy/bin:/filters/wasm \
  envoyproxy/envoy-alpine:v1.21.0
```

# 访问 envoy

```bash
curl http://localhost:8080 -I

# 返回结果
HTTP/1.1 200 OK
server: envoy
date: Sun, 24 Jul 2022 03:04:56 GMT
content-type: text/html
content-length: 612
last-modified: Thu, 29 Oct 2020 15:23:06 GMT
etag: "5f9ade5a-264"
accept-ranges: bytes
x-envoy-upstream-service-time: 0
myname: chengzw # envoy 添加的响应头
```
