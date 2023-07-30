package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

/**
* @description 手工处理队列数据
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
			fmt.Println("处理: ", item.(reconcile.Request).NamespacedName)
			time.Sleep(time.Second * 1)
			//手动模拟处理 数据
			que.Done(item)
		}
	}()
	for {
		que.Add(newItem("abc", "default"))
		time.Sleep(time.Second * 1)
		fmt.Println("塞入")

	}
}
