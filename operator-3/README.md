* [K8S 云原生 CICD 框架（Task 篇）](#k8s-云原生-cicd-框架task-篇)
    * [生成 DeepCopy 文件和 client](#生成-deepcopy-文件和-client)
    * [创建 Task CRD](#创建-task-crd)
    * [启动 operator](#启动-operator)
    * [创建 Task](#创建-task)
    * [异常处理](#异常处理)
    * [控制 step 运行顺序](#控制-step-运行顺序)
    * [支持 script 设置](#支持-script-设置)

# K8S 云原生 CICD 框架（Task 篇）

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


## 控制 step 运行顺序

初始化 Pod 时，会在 Pod 的 annotations 中添加 taskorder: "1"，准备运行第一个 step。如果当前 step 运行成功，将 taskorder 的值加 1 继续运行下一个 step，直到所有 step 执行完毕。如果中途有 step 执行失败，将 taskorder 设置为 -1。

```go
if getPod.Status.Phase == v1.PodRunning {
    //起始状态
    if getPod.Annotations[AnnotationTaskOrderKey] == AnnotationTaskOrderInitValue {
        getPod.Annotations[AnnotationTaskOrderKey] = "1" // 先运行第一个 step
        return pb.Client.Update(ctx, getPod)
    } else {
        if err := pb.forward(ctx, getPod); err != nil {
            return err
        }
    }
}
```

在 entrypoint 中会通过 --waitcontent 参数设置当 /etc/podinfo/order 文件的值等于指定 taskorder 时，开始运行这个 step。

```go
step.Container.Args = []string{
    "--wait", "/etc/podinfo/order",
    "--waitcontent", strconv.Itoa(index + 1),
    "--out", "stdout", // entrypoint 中 写上stdout 就会定向到标准输出
    "--command",
}
```

entrypoint 的等待逻辑如下。

```go
// 检查等待文件是否存在
func CheckWaitFile() {
	for {
		if _, err := os.Stat(waitFile); err == nil { //这一步代表 WaitFile存在了
			//此时 要判断WaitFileContent 是否 设置了内容，如果设置了要判断
			if waitFileContent == "" { //代表没有设置 waitFileContent ，则直接过
				return
			} else {
				getContent := getWaitContent()
				if waitFileContent == getContent { //目标一样， 则通过
					return
				}
				//此时程序要退出
				if getContent == quitContent {
					log.Println("任务被取消")
					os.Exit(1) //停止程序
				}
			}

		} else if errors.Is(err, os.ErrNotExist) { //文件真的不存在
			time.Sleep(time.Millisecond * 20)
			continue
		} else {
			log.Fatal(err) //其他 未知错误
		}
	}
}
```

## 支持 script 设置

原理是将 script 内容通过 GZIP 压缩后再通过 Base64 编码。

```go
func EncodeScript(str string) string {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte(str))
	if err != nil {
		log.Println(err)
		return ""
	}
	err = gz.Close() //这里要关掉，否则取不到数据  也可手工flush.但依然要关掉gz
	if err != nil {
		log.Println(err)
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
```

当运行的时候再解码并解压缩得到 script 内容。

```go
func GenEncodeFile(encodefile string) error {
	f, err := os.OpenFile(encodefile, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	decode := UnGzip(string(b)) //反解 之后的 字符串 ,重新写入
	err = f.Truncate(0)         //清空文件1
	if err != nil {
		return err
	}
	_, err = f.Seek(0, 0) //清空文件2
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(decode))
	if err != nil {
		if err != io.EOF {
			return err
		}
	}
	return nil
}
```