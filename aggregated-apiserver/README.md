启动本地 API Server

```bash
go run test.go
```

访问本地 API Server

```bash
kubectl --kubeconfig ./resources/config_local --insecure-skip-tls-verify=true get mi
```
