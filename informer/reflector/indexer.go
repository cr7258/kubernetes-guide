package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"myinformer/lib"
)

/**
* @description Indexer
* @author chengzw
* @since 2023/7/27
* @link
 */
func main() {
	client := lib.InitClient()
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)
	podLW := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "pods",
		"default", fields.Everything())
	df := cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{
		KeyFunction:  cache.MetaNamespaceKeyFunc,
		KnownObjects: store, // indexer 实现了 store
	})
	rf := cache.NewReflector(podLW, &v1.Pod{}, df, 0)
	ch := make(chan struct{})
	go func() {
		rf.Run(ch)
	}()
	for {
		//好比 informer 在不断消费 DeltaFIFO
		df.Pop(func(obj interface{}) error {
			for _, delta := range obj.(cache.Deltas) {
				fmt.Println(delta.Type, ":", delta.Object.(*v1.Pod).Name,
					":", delta.Object.(*v1.Pod).Status.Phase)
				switch delta.Type {
				case cache.Sync, cache.Added:
					store.Add(delta.Object)
				case cache.Updated:
					store.Update(delta.Object)
				case cache.Deleted:
					store.Delete(delta.Object)
				}

			}
			return nil
		})
	}
}
