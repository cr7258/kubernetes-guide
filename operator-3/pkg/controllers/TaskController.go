package controllers

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"operator-3/pkg/apis/task/v1alpha1"
	"operator-3/pkg/builder"
	clientset "operator-3/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type TaskController struct {
	client.Client
	*clientset.Clientset
	E record.EventRecorder //记录事件
}

func NewTaskController(e record.EventRecorder, clientset *clientset.Clientset) *TaskController {
	return &TaskController{E: e, Clientset: clientset}
}

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func (r *TaskController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	task := &v1alpha1.Task{}
	err := r.Get(ctx, req.NamespacedName, task)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = builder.NewPodBuilder(task, r.Client).Build(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func (r *TaskController) OnUpdate(event event.UpdateEvent,
	limitingInterface workqueue.RateLimitingInterface) {
	for _, ref := range event.ObjectNew.GetOwnerReferences() {
		if ref.Kind == v1alpha1.TaskKind && ref.APIVersion == v1alpha1.TaskApiVersion {
			limitingInterface.Add(reconcile.Request{
				types.NamespacedName{
					Name: ref.Name, Namespace: event.ObjectNew.GetNamespace(),
				}, //回炉
			})
		}
	}

}

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func (r *TaskController) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}
