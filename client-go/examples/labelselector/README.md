获取 label 是 app=nginx 的最新的 Configmap。

```bash
kubectl get configmaps -l app=nginx --sort-by=.metadata.creationTimestamp -o name | tail -n 1
```

