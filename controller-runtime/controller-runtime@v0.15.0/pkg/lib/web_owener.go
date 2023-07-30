package lib

import (
	"context"
	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	cc "sigs.k8s.io/controller-runtime/pkg/internal/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type MyWebOwner struct {
	hdler  handler.EventHandler
	ctl    controller.Controller
	ownObj runtime.Object
	scheme *runtime.Scheme
}

func NewMyWebOwner(hdler handler.EventHandler, ctl controller.Controller, ownobj runtime.Object, scheme *runtime.Scheme) *MyWebOwner {
	return &MyWebOwner{hdler: hdler, ctl: ctl, ownObj: ownobj, scheme: scheme}
}

func (m *MyWebOwner) Start(ctx context.Context) error {
	r := gin.New()
	r.GET("/add", func(c *gin.Context) {
		name := c.Query("name")
		cm := &v1.ConfigMap{}
		//cm.Name = name
		cm.Namespace = "mytest"

		// 设置 Configmap 的 OwnerReference 为 Pod
		m.ownObj = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
		// 执行 Reconcile 函数拿到的是 Pod 对象
		controllerutil.SetOwnerReference(m.ownObj.(metav1.Object), cm, m.scheme)

		m.hdler.Create(context.TODO(), event.CreateEvent{Object: cm},
			m.ctl.(*cc.Controller).Queue)
	})
	return r.Run(":8081")

}

var _ manager.Runnable = &MyWebOwner{}
