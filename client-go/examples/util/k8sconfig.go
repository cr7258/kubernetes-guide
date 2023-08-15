package util

import (
	"flag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

/**
* @description 初始化 clientset
* @author chengzw
* @since 2023/8/15
* @link
 */

func InitClient() (*kubernetes.Clientset, *rest.Config) {
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

	// 从本机加载 kubeconfig 配置文件，因此第一个参数为空字符串
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)

	// kubeconfig 加载失败就直接退出了
	if err != nil {
		panic(err.Error())
	}

	// 实例化 clientset 对象
	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		panic(err.Error())
	}

	return clientset, config
}
