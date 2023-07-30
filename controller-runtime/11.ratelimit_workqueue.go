package main

import (
	"fmt"
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
)

/**
* @description 限速队列
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
	que := workqueue.NewRateLimitingQueue(&workqueue.BucketRateLimiter{
		Limiter: rate.NewLimiter(1, 1), // 每秒放 1 个令牌，最多存 1 个令牌（可以设置多点支持流量突增）
	})
	go func() {
		for {
			item, _ := que.Get()
			fmt.Println(item.(reconcile.Request).NamespacedName)

			//手动模拟处理数据
			que.Done(item)
		}
	}()

	for i := 0; i < 100; i++ {
		que.AddRateLimited(newItem("abc"+strconv.Itoa(i), "default"))
	}
	select {}
}
