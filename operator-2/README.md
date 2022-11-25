## 下载 code-generator
```bash
git clone https://github.com/kubernetes/code-generator
```
## 生成 deepcopy 代码
- 第一个参数：使用那些生成器，就是 *.gen，用逗号分割，all表示使用全部
- 第二个参数：client（client-go中informer, lister等）生成的文件存放到哪里
- 第三个参数：api（api结构，k8s.io/api/） 生成的文件存放到哪里，可以和定义的文件为一个目录
- 第四个参数：定义group:version
- -output-base：输出包存放的根目录
- -go-header-file：生成文件的头注释信息，默认 code-generator 去 $GOPATH/src/k8s.io/code-generator 目录下找 boilerplate 文件，可以使用该参数设置
```bash
code-generator/generate-groups.sh all \
github.com/shenyisyn/dbcore/pkg/client \
github.com/shenyisyn/dbcore/pkg/apis dbconfig:v1 \
--output-base ./ \
--go-header-file code-generator/hack/boilerplate.go.txt
```
参考：
https://www.cnblogs.com/Cylon/p/16394839.html
https://github.com/rook/rook/issues/5083

## 设置属性
- CRD 说明：
https://kubernetes.io/zh/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#validation
- OpenAPI 规范文档：
https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.0.0.md

## 设置显示的列
```yaml
additionalPrinterColumns:
- name: replicas
  type: string
  jsonPath: .spec.status.replicas
- name: Age
  type: date
  jsonPath: .metadata.creationTimestamp
```

## 伸缩属性设置
```yaml
subresources:
status: {}
scale:
  # specReplicasPath 定义定制资源中对应 scale.spec.replicas 的 JSON 路径
  specReplicasPath: .spec.replicas
  # statusReplicasPath 定义定制资源中对应 scale.status.replicas 的 JSON 路径
  statusReplicasPath: .status.replicas
```

扩缩容
```bash
kubectl scale --replicas=3 dc/mydbconfig
```

## 级联删除

设置 OwnerReferences 属性。删除我们自定义的 DC 对象时，会同时删除关联的 Deployment 对象。

```go
this.deploy.OwnerReferences=append(this.deploy.OwnerReferences,
	  	v1.OwnerReference{
	  	   APIVersion: this.config.APIVersion,
	  	   Kind:this.config.Kind,
	  	   Name: this.config.Name,
			UID:this.config.UID,
		})
```

https://kubernetes.io/zh-cn/docs/concepts/architecture/garbage-collection/

## 重新拉起被手工删掉的资源

监听 Deployment 对象的 OnDelete 事件，当 Deployment 被删除时，重新放入 Reconcile。 
```go
func (r *DbConfigController) OnDelete(event event.DeleteEvent, limitingInterface workqueue.RateLimitingInterface) {
	for _, ref := range event.Object.GetOwnerReferences() {
		if ref.Kind == "DbConfig" && ref.APIVersion == "api.jtthink.com/v1" {
			limitingInterface.Add(
				reconcile.Request{ // 重新把对象放入 reconcile
					types.NamespacedName{
						Name: ref.Name, Namespace: event.Object.GetNamespace(),
					},
				})
		}
	}
}
```