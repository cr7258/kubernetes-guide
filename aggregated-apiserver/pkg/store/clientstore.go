package store

import (
	"fmt"
	"github.com/shenyisyn/aapi/pkg/apis/myingress/v1beta1"
	"github.com/shenyisyn/aapi/pkg/k8sconfig"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type ClientStore struct {
}

func NewClientStore() *ClientStore {
	return &ClientStore{}
}
func (cs *ClientStore) GetByNs(name, ns string) (*v1beta1.MyIngress, error) {
	ingress, err := k8sconfig.Factory.Networking().V1().Ingresses().Lister().Ingresses(ns).
		Get(name)
	if err != nil {
		return nil, err
	}
	mi := &v1beta1.MyIngress{
		ObjectMeta: ingress.ObjectMeta,
		Spec: v1beta1.MyIngressSpec{
			Host: ingress.Spec.Rules[0].Host,
			Path: ingress.Spec.Rules[0].HTTP.Paths[0].Path,
			Service: fmt.Sprintf("%s:%d", ingress.Spec.Rules[0].HTTP.Paths[0].
				Backend.Service.Name, ingress.Spec.Rules[0].HTTP.Paths[0].
				Backend.Service.Port.Number),
		},
	}
	mi.Kind = v1beta1.ResourceKind
	mi.APIVersion = v1beta1.ApiGroupAndVersion
	return mi, nil
}

//根据ns来获取Ingress列表， 并转化为 我们自己的资源 MyIngressList
//改造了代码。 实现了  全部加载。 代码很简单，大家自己看看
func (cs *ClientStore) ListByNsOrAll(ns string) (*v1beta1.MyIngressList, error) {
	lister := k8sconfig.Factory.Networking().V1().Ingresses().Lister()
	var list []*v1.Ingress
	if ns != "" { //不限定ns
		_list, err := lister.Ingresses(ns).
			List(labels.Everything())
		if err != nil {
			return nil, err
		}
		list = _list
	} else {
		_list, err := lister.
			List(labels.Everything())
		if err != nil {
			return nil, err
		}
		list = _list
	}
	myList := &v1beta1.MyIngressList{}
	for _, ingress := range list {
		myList.Items = append(myList.Items, v1beta1.MyIngress{
			TypeMeta:   ingress.TypeMeta,
			ObjectMeta: ingress.ObjectMeta,
			Spec: v1beta1.MyIngressSpec{
				Host:    ingress.Spec.Rules[0].Host,
				Path:    ingress.Spec.Rules[0].HTTP.Paths[0].Path,
				Service: ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.String(),
			},
		})
	}
	return myList, nil

}
