package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"myinformer/lib"
	"myinformer/watchdog"
)

/**
* @description 实现简单的 Informer
* @author chengzw
* @since 2023/7/27
* @link
 */

type PodHandler struct{}

func (p *PodHandler) OnAdd(obj interface{}) {
	fmt.Println("OnAdd:", obj.(*v1.Pod).Name)
}

func (p *PodHandler) OnUpdate(oldObj, newObj interface{}) {
	fmt.Println("OnUpdate:", newObj.(*v1.Pod).Name)
}

func (p *PodHandler) OnDelete(obj interface{}) {
	fmt.Println("OnDelete:", obj.(*v1.Pod).Name)
}

var _ cache.ResourceEventHandler = &PodHandler{}

func main() {
	client := lib.InitClient()
	podLW := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "pods", "default", fields.Everything())
	wd := watchdog.NewWatchdog(podLW, &v1.Pod{}, &PodHandler{})
	ch := make(chan struct{})
	wd.Run(ch)
}
