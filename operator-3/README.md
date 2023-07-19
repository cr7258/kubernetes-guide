## 生成 DeepCopy 文件和 client

下载 code-generator v0.23.0 版本，和我们的 k8s.io 包的版本一致：https://github.com/kubernetes/code-generator/releases/tag/v0.23.0

```bash
code-generator/generate-groups.sh all \
# 指定 group 和 version，生成 deeplycopy 以及 client
operator-3/pkg/client operator-3/pkg/apis task:v1alpha1 \
# 指定头文件
--go-header-file=./code-generator/hack/boilerplate.go.txt \
# 指定输出位置，默认为GOPATH
--output-base ./
```