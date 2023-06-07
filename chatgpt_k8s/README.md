## 快速集成 ChatGPT 客户端库

```bash
cd quickstart
go run main.go

# 输出，回答的结果可能不是很准确
Kubernetes is a cloud-scale infrastructure platform that lets opsky owners connect to Kubernetes.
```

## 上下文连续对话代码编写

```bash
cd quickstart
go run continuous_dialogue.go

# 输出
生成一个 k8s pod yaml 文件

apiVersion: v1
kind: Pod
metadata:
name: my-pod
labels:
app: my-app
spec:
containers:
- name: my-container
  image: nginx:1.17.3
  ports:
    - containerPort: 80

请把副本调整为5
 
apiVersion: v1
kind: Pod
metadata:
name: my-pod
labels:
app: my-app
spec:
replicas: 5
containers:
- name: my-container
  image: nginx:1.17.3
  ports:
    - containerPort: 80
```