app.yaml 文件初始配置，只需要启动端口的配置。
```bash
Server:
  Port: 80
```
更新 Kubernetes Ingress 资源配置。
```bash
kubectl apply -f resources/testingress.yaml
```
控制器会自动重载 app.yaml 配置文件。
```yaml
Ingress:
- apiVersion: networking.k8s.io/v1
  kind: Ingress
  metadata:
    annotations:
      jtthink.ingress.kubernetes.io/add-response-header: ret=okabc
      kubectl.kubernetes.io/last-applied-configuration: |
        {"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"annotations":{"jtthink.ingress.kubernetes.io/add-response-header":"ret=okabc","kubernetes.io/ingress.class":"jtthink"},"name":"ingress-myservicea","namespace":"default"},"spec":{"rules":[{"host":"aabb.com","http":{"paths":[{"backend":{"service":{"name":"jtapp","port":{"number":8080}}},"path":"/","pathType":"Prefix"}]}}]}}
      kubernetes.io/ingress.class: jtthink
    creationTimestamp: "2022-08-25T13:09:07Z"
    generation: 1
    managedFields:
    - apiVersion: networking.k8s.io/v1
      fieldsType: FieldsV1
      fieldsV1:
        f:metadata:
          f:annotations:
            .: {}
            f:jtthink.ingress.kubernetes.io/add-response-header: {}
            f:kubectl.kubernetes.io/last-applied-configuration: {}
            f:kubernetes.io/ingress.class: {}
        f:spec:
          f:rules: {}
      manager: kubectl-client-side-apply
      operation: Update
      time: "2022-08-25T13:09:07Z"
    name: ingress-myservicea
    namespace: default
    resourceVersion: "130672"
    uid: 7ffdff92-c824-4de4-b978-75e3fd6f3f68
  spec:
    rules:
    - host: aabb.com
      http:
        paths:
        - backend:
            service:
              name: jtapp
              port:
                number: 8080
          path: /
          pathType: Prefix
  status:
    loadBalancer: {}
Server:
  Port: 80
```
打包镜像。
```bash
docker build -t cr7258/jtproxy:v1 .
docker push  cr7258/jtproxy:v1
```

部署 jtproxy 和后端服务（upstream）到 Kubernetes 集群。
```bash
kubectl apply -f deploy
```

开启端口转发，便于本地测试。
```bash
kubectl port-forward -n jtthink-system service/jtproxy-svc  8888:80
```
配置 host 记录。
```
127.0.0.1  aabb.com
```

本地访问，应该可以访问到后端的 nginx。修改 testingress.yaml 文件 jtproxy 可以动态重置配置。
```
curl aabb.com:8888
```
