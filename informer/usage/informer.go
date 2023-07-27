package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"myinformer/lib"
)

/**
* @description Informer 使用方法
* @author chengzw
* @since 2023/7/27
* @link
 */
func main() {
	// 创建 Kubernetes客户端
	client := lib.InitClient()

	// 创建 Pod 的 Informer
	podLW := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "pods", "default", fields.Everything())
	_, informer := cache.NewInformer(
		podLW, &v1.Pod{}, 0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { fmt.Printf("add: %v\n", obj.(*v1.Pod).Name) },
			UpdateFunc: func(oldObj, newObj interface{}) { fmt.Printf("update: %v\n", newObj.(*v1.Pod).Name) },
			DeleteFunc: func(obj interface{}) { fmt.Printf("delete: %v\n", obj.(*v1.Pod).Name) },
		})

	// 启动 Informer
	informer.Run(wait.NeverStop)
}
