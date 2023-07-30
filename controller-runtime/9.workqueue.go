package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

/**
* @description 工作队列
* @author chengzw
* @since 2023/7/30
* @link
 */

func newItem(name, ns string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      name,
		Namespace: ns,
	}}
}

func main() {
	que := workqueue.New()
	go func() {
		for {
			item, _ := que.Get()
			fmt.Println(item.(reconcile.Request).NamespacedName)
			time.Sleep(time.Millisecond * 20)
		}
	}()
	for {
		que.Add(newItem("abc", "default"))
		que.Add(newItem("abc", "default"))
		que.Add(newItem("abc2", "default"))
		time.Sleep(time.Second * 1)
	}
}
