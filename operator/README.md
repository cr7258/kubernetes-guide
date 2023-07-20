## 生成代码

```bash
go mod tidy
go mod vendor

./hack/update-codegen.sh

# 输出结果
Generating deepcopy funcs
Generating clientset for foo:v1alpha1 at my-operator/pkg/custom/client/clientset
Generating listers for foo:v1alpha1 at my-operator/pkg/custom/client/listers
Generating informers for foo:v1alpha1 at my-operator/pkg/custom/client/informers
```

会在 pkg 目录下生成 DeepCopy 文件以及 client 目录。

![](https://chengzw258.oss-cn-beijing.aliyuncs.com/Article/20230720142432.png)

## 参考资料

- [Kubernetes Controller 机制详解（一）](https://mp.weixin.qq.com/s/TyA1bNXLQs1mXzZbw2PHfw)
- [Kubernetes Controller 机制详解（二）](https://mp.weixin.qq.com/s/SNOY7dOl2MBBe_XIgfgnCg)