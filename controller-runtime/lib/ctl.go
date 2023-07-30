package lib

import (
	"context"
	"fmt"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

type Ctl struct {
}

func (*Ctl) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	fmt.Println(req.NamespacedName)
	return controllerruntime.Result{}, nil
}
