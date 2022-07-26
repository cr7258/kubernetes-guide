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