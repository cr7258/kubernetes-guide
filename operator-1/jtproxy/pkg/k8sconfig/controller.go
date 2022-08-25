package k8sconfig

import (
	"context"
	"jtproxy/pkg/sysinit"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/util/workqueue"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//@Controller
type JtProxyController struct {
	client.Client
}

func NewJtProxyController() *JtProxyController {
	return &JtProxyController{}
}
func (r *JtProxyController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ingress := &v1.Ingress{}

	err := r.Get(ctx, req.NamespacedName, ingress)
	if err != nil {
		return reconcile.Result{}, err
	}
	if r.IsJtProxy(ingress.Annotations) {
		err = sysinit.ApplyConfig(ingress)
		if err != nil {
			return reconcile.Result{}, err
		}
		if ingress.Status.LoadBalancer.Ingress == nil {
			ingress.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{IP: ServiceIP},
			}
			err = r.Status().Update(ctx, ingress)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return reconcile.Result{}, nil
}
func (r *JtProxyController) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}

//判断是否 是否 我们所需要处理的ingress
func (r *JtProxyController) IsJtProxy(annotations map[string]string) bool {
	if v, ok := annotations["kubernetes.io/ingress.class"]; ok && v == "jtthink" {
		return true
	}
	return false
}
func (r *JtProxyController) OnDelete(event event.DeleteEvent, limitingInterface workqueue.RateLimitingInterface) {
	if r.IsJtProxy(event.Object.GetAnnotations()) {
		if err := sysinit.DeleteConfig(event.Object.GetName(), event.Object.GetNamespace()); err != nil {
			log.Println(err)
		}
	}
}
