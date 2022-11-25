package controllers

import (
	"context"
	v1 "github.com/shenyisyn/dbcore/pkg/apis/dbconfig/v1"
	"github.com/shenyisyn/dbcore/pkg/builders"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type DbConfigController struct {
	client.Client
}

func NewDbConfigController() *DbConfigController {
	return &DbConfigController{}
}

func (r *DbConfigController) OnDelete(event event.DeleteEvent, limitingInterface workqueue.RateLimitingInterface) {
	for _, ref := range event.Object.GetOwnerReferences() {
		if ref.Kind == "DbConfig" && ref.APIVersion == "api.jtthink.com/v1" {
			limitingInterface.Add(
				reconcile.Request{ // 重新把对象放入 reconcile
					types.NamespacedName{
						Name: ref.Name, Namespace: event.Object.GetNamespace(),
					},
				})
		}
	}
}

func (r *DbConfigController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	config := &v1.DbConfig{}
	err := r.Get(ctx, req.NamespacedName, config)
	if err != nil {
		return reconcile.Result{}, err
	}
	builder, err := builders.NewDeployBuilder(config, r.Client)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = builder.Build(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, err
}
func (r *DbConfigController) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}
