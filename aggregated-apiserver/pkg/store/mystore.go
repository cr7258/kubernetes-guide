package store

import (
	"context"
	"github.com/shenyisyn/aapi/pkg/apis/myingress/v1beta1"
	"reflect"
	"sync"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

//type REST struct {
//	*genericregistry.Store
//}
var _ rest.StandardStorage = &store{}
var _ rest.Scoper = &store{}
var _ rest.Storage = &store{}

func NewMyStore(groupResource schema.GroupResource, isNamespaced bool,
	tc rest.TableConvertor) rest.Storage {

	return &store{
		defaultQualifiedResource: groupResource,
		TableConvertor:           tc,
		isNamespaced:             isNamespaced,
		newFunc: func() runtime.Object {
			return &v1beta1.MyIngress{}
		},
		newListFunc: func() runtime.Object {
			return &v1beta1.MyIngressList{}
		},
	}

}

type store struct {
	rest.TableConvertor
	isNamespaced bool
	muWatchers   sync.RWMutex

	newFunc                  func() runtime.Object
	newListFunc              func() runtime.Object
	defaultQualifiedResource schema.GroupResource
}

func (f *store) notifyWatchers(ev watch.Event) {

}

func (f *store) New() runtime.Object {
	return f.newFunc()
}

func (f *store) NewList() runtime.Object {
	return f.newListFunc()
}

func (f *store) NamespaceScoped() bool {
	return f.isNamespaced
}
func (f *store) ShortNames() []string {
	return []string{"mi"}
}

func (f *store) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	test := &v1beta1.MyIngress{}
	test.Namespace = "default"
	test.Name = "test"
	return test, nil

}

func (f *store) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	newListObj := f.NewList()
	return newListObj, nil
}

func (f *store) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc,
	options *metav1.CreateOptions) (runtime.Object, error) {

	return obj, nil
}

func (f *store) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {

	oldObj, _ := f.Get(ctx, name, nil)

	return oldObj, false, nil
}

func (f *store) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions) (runtime.Object, bool, error) {

	oldObj, err := f.Get(ctx, name, nil)
	if err != nil {
		return nil, false, err
	}

	return oldObj, true, nil
}

func (f *store) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions, listOptions *metainternalversion.ListOptions) (runtime.Object, error) {
	newListObj := f.NewList()

	return newListObj, nil
}

func appendItem(v reflect.Value, obj runtime.Object) {
	v.Set(reflect.Append(v, reflect.ValueOf(obj).Elem()))
}

func (f *store) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {

	return nil, nil
}

func (f *store) ConvertToTable(ctx context.Context, object runtime.Object,
	tableOptions runtime.Object) (*metav1.Table, error) {
	return f.TableConvertor.ConvertToTable(ctx, object, tableOptions)
}
func (f *store) qualifiedResourceFromContext(ctx context.Context) schema.GroupResource {
	if info, ok := genericapirequest.RequestInfoFrom(ctx); ok {
		return schema.GroupResource{Group: info.APIGroup, Resource: info.Resource}
	}
	// some implementations access storage directly and thus the context has no RequestInfo
	return f.defaultQualifiedResource
}
