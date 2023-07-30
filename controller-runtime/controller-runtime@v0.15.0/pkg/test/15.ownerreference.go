package main

import (
	"context"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/lib"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

/**
* @description Owner 资源监听
* @author chengzw
* @since 2023/7/30
* @link
 */

func main() {

	mgr, err := manager.New(lib.K8sRestConfig(),
		manager.Options{
			Logger:    logf.Log.WithName("test"),
			Namespace: "default",
		})

	lib.Check(err)
	// 这里我们手工创建的控制器
	ctl, err := controller.New("abc", mgr, controller.Options{
		Reconciler: &lib.Ctl{}, // struct 需要实现 Reconcile 方法
	})
	lib.Check(err)

	src := source.Kind(mgr.GetCache(), &v1.Pod{})
	ownObj := &v1.Pod{}
	hdler := handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), ownObj)
	lib.Check(ctl.Watch(src, hdler))

	lib.Check(mgr.Add(lib.NewMyWebOwner(hdler, ctl, ownObj, mgr.GetScheme())))

	err = mgr.Start(context.Background())
	lib.Check(err)

}
