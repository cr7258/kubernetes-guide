package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig *string

	// home 是家目录，如果能取得家目录的值，就可以用来做默认值
	if home := homedir.HomeDir(); home != "" {
		// 如果输入了 kubeconfig 参数，该参数的值就是 kubeconfig 文件的绝对路径，
		// 如果没有输入 kubeconfig 参数，就用默认路径 ~/.kube/config
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		// 如果取不到当前用户的家目录，就没办法设置 kubeconfig 的默认目录了，只能从入参中取
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

	// 从本机加载 kubeconfig 配置文件，因此第一个参数为空字符串
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)

	// kubeconfig 加载失败就直接退出了
	if err != nil {
		panic(err.Error())
	}

	// 参考 path : /api/v1/namespaces/{namespace}/pods
	config.APIPath = "api"
	// pod 的 group 是空字符串
	config.GroupVersion = &corev1.SchemeGroupVersion
	// 指定序列化工具
	config.NegotiatedSerializer = scheme.Codecs

	// 根据配置信息构建 restClient 实例
	restClient, err := rest.RESTClientFor(config)

	if err != nil {
		panic(err.Error())
	}

	// 保存 pod 结果的数据结构实例
	result := &corev1.PodList{}

	//  指定 namespace
	namespace := "kube-system"
	// 设置请求参数，然后发起请求
	// GET 请求
	err = restClient.Get().
		//  指定 namespace，参考 path : /api/v1/namespaces/{namespace}/pods
		Namespace(namespace).
		// 查找多个 pod，参考 path : /api/v1/namespaces/{namespace}/pods
		Resource("pods").
		// 指定大小限制和序列化工具
		VersionedParams(&metav1.ListOptions{Limit: 100}, scheme.ParameterCodec).
		// 请求
		Do(context.TODO()).
		// 结果存入 result
		Into(result)

	if err != nil {
		panic(err.Error())
	}

	// 表头
	fmt.Printf("namespace\t status\t\t name\n")

	// 每个pod都打印 namespace、status.Phase、name 三个字段
	for _, d := range result.Items {
		fmt.Printf("%v\t %v\t %v\n",
			d.Namespace,
			d.Status.Phase,
			d.Name)
	}
}
