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
* @description SharedInformer 使用方法
* @author chengzw
* @since 2023/7/27
* @link
 */

func main() {
	// 创建 Kubernetes客户端
	client := lib.InitClient()

	// 创建 Pod 的 SharedInformer
	podLW := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "pods", "default", fields.Everything())
	informer := cache.NewSharedInformer(
		podLW, &v1.Pod{}, 0, // resyncPeriod, 0 表示不主动触发重新同步
	)

	// 注册事件处理函数，SharedInformer 可以添加多个 EventHandler
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { fmt.Printf("add: %v\n", obj.(*v1.Pod).Name) },
		UpdateFunc: func(oldObj, newObj interface{}) { fmt.Printf("update: %v\n", newObj.(*v1.Pod).Name) },
		DeleteFunc: func(obj interface{}) { fmt.Printf("delete: %v\n", obj.(*v1.Pod).Name) },
	})

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { fmt.Printf("add-2: %v\n", obj.(*v1.Pod).Name) },
		UpdateFunc: func(oldObj, newObj interface{}) { fmt.Printf("update-2: %v\n", newObj.(*v1.Pod).Name) },
		DeleteFunc: func(obj interface{}) { fmt.Printf("delete-2: %v\n", obj.(*v1.Pod).Name) },
	})

	// 启动 SharedInformer
	informer.Run(wait.NeverStop)
}
