package helper

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func K8sClient() *kubernetes.Clientset {
	config := &rest.Config{
		Host: "https://127.0.0.1:6443",
	}
	config.Insecure = true

	// 创建一个新的Kubernetes客户端
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return clientset
}
