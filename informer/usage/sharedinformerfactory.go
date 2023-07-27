package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"myinformer/lib"
	"time"
)

/**
* @description SharedInformerFactory 使用方法
* @author chengzw
* @since 2023/7/27
* @link
 */

func main() {

	client := lib.InitClient()
	// 生成SharedInformerFactory
	factory := informers.NewSharedInformerFactory(client, 5*time.Second)
	// 生成 PodInformer
	podInformer := factory.Core().V1().Pods().Informer()
	//注册 add, update, del 处理事件
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { fmt.Printf("add: %v\n", obj.(*v1.Pod).Name) },
		UpdateFunc: func(oldObj, newObj interface{}) { fmt.Printf("update: %v\n", newObj.(*v1.Pod).Name) },
		DeleteFunc: func(obj interface{}) { fmt.Printf("delete: %v\n", obj.(*v1.Pod).Name) },
	})

	// 生成 ServiceInformer
	serviceInformer := factory.Core().V1().Services().Informer()
	//注册 add, update, del 处理事件
	serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { fmt.Printf("add: %v\n", obj.(*v1.Service).Name) },
		UpdateFunc: func(oldObj, newObj interface{}) { fmt.Printf("update: %v\n", newObj.(*v1.Service).Name) },
		DeleteFunc: func(obj interface{}) { fmt.Printf("delete: %v\n", obj.(*v1.Service).Name) },
	})

	stopCh := make(chan struct{})
	// 启动 factory 下面所有的 informer
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	//pods, _ := podInformer.Lister().Pods("default").List(labels.Everything())
	//
	//for _, p := range pods {
	//	fmt.Printf("list pods: %v\n", p.Name)
	//}
	<-stopCh
}
