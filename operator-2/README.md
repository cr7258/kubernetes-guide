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