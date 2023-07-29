package main

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"my-controller-runtime/lib"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"time"
)

/**
* @description 查看 Manager 中的 Informer
* @author chengzw
* @since 2023/7/29
* @link
 */
func main() {
	mgr, err := manager.New(lib.K8sRestConfig(),
		manager.Options{
			Logger:    logf.Log.WithName("test"),
			Namespace: "default", // 只监控 default namespace 下的资源
		})

	lib.Check(err)
	go func() {
		time.Sleep(time.Second * 2)
		podInformer, _ := mgr.GetCache().GetInformer(context.Background(), &v1.Pod{})
		fmt.Println(podInformer.(cache.SharedIndexInformer).GetIndexer().ListKeys())
	}()

	err = mgr.Start(context.Background())
	lib.Check(err)
}
