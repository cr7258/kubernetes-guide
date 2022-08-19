/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"jtapp/helper"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	myappv1 "jtapp/api/v1"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=myapp.jtthink.com,resources=redis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=myapp.jtthink.com,resources=redis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=myapp.jtthink.com,resources=redis/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Redis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	redis := &myappv1.Redis{}
	if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
		return ctrl.Result{}, err
	} else {
		// 正在删除 redis 对象，删除之前先清除关联的 pod
		if !redis.DeletionTimestamp.IsZero() {
			return ctrl.Result{}, r.clearRedis(ctx, redis)
		}
		// 创建
		podNames := helper.GetRedisPodNames(redis)
		isEdit := false
		// 遍历创建 pod，挨个创建，如果已经创建则不做处理
		for _, po := range podNames {
			podName, err := helper.CreateRedis(r.Client, redis, po)
			if err != nil {
				return ctrl.Result{}, err
			}
			if podName == "" { // 在 Finalizers 已经存在 redis pod
				continue
			}
			redis.Finalizers = append(redis.Finalizers, podName)
			isEdit = true
		}

		//收缩 福本
		if len(redis.Finalizers) > len(podNames) {
			isEdit = true
			err := r.rmIfSurplus(ctx, podNames, redis)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		// 是否发生了 pod 创建/收缩，如果没发生，就没必要 update 资源
		if isEdit {
			err = r.Client.Update(ctx, redis)
			return ctrl.Result{}, err
		} else {
			return ctrl.Result{}, nil
		}
	}
}

// 收缩副本  ['redis0','redis1']   ---> podName ['redis0']
func (r *RedisReconciler) rmIfSurplus(ctx context.Context, poNames []string, redis *myappv1.Redis) error {
	for i := 0; i < len(redis.Finalizers)-len(poNames); i++ {
		err := r.Client.Delete(ctx, &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: redis.Finalizers[len(poNames)+i], Namespace: redis.Namespace},
		})
		if err != nil {
			return err
		}
	}
	redis.Finalizers = poNames

	return nil
}

func (r *RedisReconciler) clearRedis(ctx context.Context, redis *myappv1.Redis) error {
	podList := redis.Finalizers
	for _, podName := range podList {
		err := r.Client.Delete(ctx, &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: redis.Namespace},
		})
		if err != nil {
			return err
		}
	}
	redis.Finalizers = []string{}
	return r.Client.Update(ctx, redis)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&myappv1.Redis{}).
		Complete(r)
}
