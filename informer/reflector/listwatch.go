package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"log"
	"myinformer/lib"
)

/**
* @description 手工创建 List & Watch 监听
* @author chengzw
* @since 2023/7/27
* @link
 */

func main() {
	client := lib.InitClient()

	podLW := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "pods", "default", fields.Everything())

	// List
	list, err := podLW.List(metav1.ListOptions{})
	if err != nil {
		log.Fatalln(err)
	}
	podList := list.(*v1.PodList)
	for _, pod := range podList.Items {
		fmt.Println(pod.Name)
	}

	// Watch
	watcher, err := podLW.Watch(metav1.ListOptions{})
	if err != nil {
		log.Fatalln(err)
	}
	for {
		select {
		case v, ok := <-watcher.ResultChan():
			if ok {
				fmt.Println(v.Type, ":", v.Object.(*v1.Pod).Name, ", Status: ", v.Object.(*v1.Pod).Status.Phase)
			}
		}
	}
}
