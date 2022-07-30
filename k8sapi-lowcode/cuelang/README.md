## 渲染模板

```bash
cue export test.cue   
cue export test.cue -e pod
cue export test.cue -e pod --out yaml

```


渲染效果如下：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
  namespace: abc
spec:
  containers:
    - image: nginx:1.18-alpine
    - image: tomcat
      name: myapp
```

快速生成 nginx 配置：
```bash
cue export nginx.cue -e output -o nginx.yaml
```

## 接受客户端传入参数渲染模板

```bash
curl -XPOST http://localhost:8080 -H "content-type: application/json" \
-d '{
  "apiVersion": "v1",
  "kind": "Pod",
  "name": "abc"
}'

# 返回结果
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "name": "nginx",
    "namespace": "abc"
  },
  "spec": {
    "containers": [
      {
        "image": "nginx:1.18-alpine"
      },
      {
        "image": "tomcat",
        "name": "myapp"
      }
    ]
  }
}
```