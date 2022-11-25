package controllers

import (
	"context"
	"fmt"
	v1 "github.com/shenyisyn/dbcore/pkg/apis/dbconfig/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type DbConfigController struct {
	client.Client
}

func NewDbConfigController() *DbConfigController {
	return &DbConfigController{}
}
func (r *DbConfigController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	config := &v1.DbConfig{}
	err := r.Get(ctx, req.NamespacedName, config)
	if err != nil {
		return reconcile.Result{}, err
	}
	fmt.Println(config)
	return reconcile.Result{}, err
}
func (r *DbConfigController) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}
