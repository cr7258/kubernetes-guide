## 生成 DeepCopy 文件和 client

下载 code-generator v0.23.0 版本，和我们的 k8s.io 包的版本一致：https://github.com/kubernetes/code-generator/releases/tag/v0.23.0

```bash
code-generator/generate-groups.sh all \
operator-3/pkg/client operator-3/pkg/apis task:v1alpha1 \
--go-header-file=./code-generator/hack/boilerplate.go.txt \
--output-base ./

# 输出结果
Generating deepcopy funcs
Generating clientset for task:v1alpha1 at operator-3/pkg/client/clientset
Generating listers for task:v1alpha1 at operator-3/pkg/client/listers
Generating informers for task:v1alpha1 at operator-3/pkg/client/informers
```

参考资料：https://juejin.cn/post/7096484178128011277

然后把 operator-3 目录下的文件拷贝到 pkg 目录中。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230719082630.png)

## 创建 Task CRD

```bash
kubectl apply -f crd/task.yaml
```

## 启动 operator

```bash
go run main.go
```

## 创建 Task

```bash
kubectl apply -f crd/t1.yaml
```