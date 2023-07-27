package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"myinformer/lib"
)

/**
* @description 手工创建 Reflector 和队列取值
* @author chengzw
* @since 2023/7/27
* @link
 */

func main() {
	client := lib.InitClient()
	podLW := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "pods", "default", fields.Everything())
	df := cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{KeyFunction: cache.MetaNamespaceKeyFunc})
	rf := cache.NewReflector(podLW, &v1.Pod{}, df, 0)

	ch := make(chan struct{})
	go func() {
		rf.Run(ch)
	}()

	for {
		df.Pop(func(obj interface{}) error {
			for _, delta := range obj.(cache.Deltas) {
				fmt.Println(delta.Type, ":", delta.Object.(*v1.Pod).Name,
					":", delta.Object.(*v1.Pod).Status.Phase)
			}
			return nil
		})
	}
}
