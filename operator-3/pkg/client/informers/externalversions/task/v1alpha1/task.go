/*
Copyright The Kubernetes Authors.

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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	taskv1alpha1 "operator-3/pkg/apis/task/v1alpha1"
	versioned "operator-3/pkg/client/clientset/versioned"
	internalinterfaces "operator-3/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "operator-3/pkg/client/listers/task/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// TaskInformer provides access to a shared informer and lister for
// Tasks.
type TaskInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.TaskLister
}

type taskInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewTaskInformer constructs a new informer for Task type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewTaskInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredTaskInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredTaskInformer constructs a new informer for Task type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredTaskInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ApiV1alpha1().Tasks(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ApiV1alpha1().Tasks(namespace).Watch(context.TODO(), options)
			},
		},
		&taskv1alpha1.Task{},
		resyncPeriod,
		indexers,
	)
}

func (f *taskInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredTaskInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *taskInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&taskv1alpha1.Task{}, f.defaultInformer)
}

func (f *taskInformer) Lister() v1alpha1.TaskLister {
	return v1alpha1.NewTaskLister(f.Informer().GetIndexer())
}
