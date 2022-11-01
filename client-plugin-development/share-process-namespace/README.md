build app
```bash
export GOOS=linux
export GOARCH=amd64

go build -o app1 app1.go
go build -o app2 app2.go
```

upload the app1 and app2 executable file to the k8s node, then execute kubectl apply command to the deploy.yaml file.
```bash
> kubectl get pod
NAME                        READY   STATUS    RESTARTS   AGE
k8splay1-6784b6cb56-r66hd   2/2     Running   0          73s
```

get counter.
```bash
kubectl exec -it k8splay1-6784b6cb56-r66hd -c k8splayapp1 -- curl localhost:8080/counter
```

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20221101122843.png)

reset the counter.
```bash
# app2 will send reset signal to app1
> kubectl exec -it k8splay1-6784b6cb56-r66hd -c k8splayapp2 -- curl localhost:8081/reset
command terminated with exit code 138

# view app1 logs
> kubectl logs k8splay1-6784b6cb56-r66hd -c k8splayapp1
counter is reset

# the counter was reset 
> kubectl exec -it k8splay1-6784b6cb56-r66hd -c k8splayapp1 -- curl localhost:8080/counter
counter is 1
```