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
* @description 手工触发 Reconcile 函数
* @author chengzw
* @since 2023/7/29
* @link
 */
func main() {
	mgr, err := manager.New(lib.K8sRestConfig(),
		manager.Options{
			Logger:    logf.Log.WithName("test"),
			Namespace: "default",
		})
	lib.Check(err)

	// 会通过 mgr.Add(c) 将 controller 添加到 runnables 中
	ctl, err := controller.New("abc", mgr, controller.Options{
		Reconciler: &lib.Ctl{}, // struct 需要实现 Reconcile 方法
	})
	lib.Check(err)

	src := source.Kind(mgr.GetCache(), &v1.Pod{})
	hdler := &handler.EnqueueRequestForObject{}
	lib.Check(ctl.Watch(src, hdler))

	// 将 controller 添加到 runnables 中
	myweb := lib.NewMyWeb(hdler, ctl)
	mgr.Add(myweb)

	err = mgr.Start(context.Background())
	lib.Check(err)
}
