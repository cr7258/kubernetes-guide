package lib

import (
	"context"
	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	cc "sigs.k8s.io/controller-runtime/pkg/internal/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type MyWeb struct {
	h   handler.EventHandler
	ctl controller.Controller
}

func NewMyWeb(h handler.EventHandler, ctl controller.Controller) *MyWeb {
	return &MyWeb{h: h, ctl: ctl}
}

// 实现 Runnable 接口
func (m *MyWeb) Start(ctx context.Context) error {
	r := gin.New()
	r.GET("/add", func(c *gin.Context) {
		pod := &v1.Pod{}
		pod.Name = "my-pod"
		pod.Namespace = "my-namespace"
		// 往 Queue 中添加 Event 来手动触发 Reconcile 函数
		m.h.Create(context.TODO(), event.CreateEvent{Object: pod}, m.ctl.(*cc.Controller).Queue)
	})
	return r.Run(":8081")
}

var _ manager.Runnable = &MyWeb{}
