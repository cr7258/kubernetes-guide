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

# 返回结果
step1 用的是普通模式
step2 用的是普通模式
step3 用的是普通模式
step4 用的是script模式
新状态 Pending
order是 0
新状态 Pending
order是 0
新状态 Pending
order是 0
新状态 Running
order是 1
新状态 Running
order是 2
新状态 Running
order是 2
新状态 Running
order是 3
新状态 Running
order是 3
新状态 Running
order是 4
新状态 Running
order是 4
新状态 Running
order是 4
新状态 Running
order是 4
新状态 Succeeded
order是 4
```

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230719205436.png)

查看 Pod 的日志

## 创建 Task

```bash
kubectl apply -f crd/t1.yaml
```

## 异常处理

修改 step2 任务的退出码为 exit 1，让任务异常退出，然后更新 Task。

```bash
kubectl apply -f crd/t1.yaml
```

查看 Controller 日志。

```bash
step1 用的是普通模式
step2 用的是普通模式
step3 用的是普通模式
step4 用的是script模式
新状态 Pending
order是 0
新状态 Pending
order是 0
新状态 Pending
order是 0
新状态 Pending
order是 0
新状态 Running
order是 1
新状态 Running
order是 2
新状态 Running
order是 2
新状态 Running
order是 -1
新状态 Running
order是 -1
新状态 Running
order是 -1
新状态 Running
order是 -1
新状态 Failed
order是 -1
```

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230719205202.png)