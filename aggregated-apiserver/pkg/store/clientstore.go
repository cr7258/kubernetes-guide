package store

import (
	"github.com/shenyisyn/aapi/pkg/apis/myingress/v1beta1"
	"github.com/shenyisyn/aapi/pkg/k8sconfig"
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
			Host:    ingress.Spec.Rules[0].Host,
			Path:    ingress.Spec.Rules[0].HTTP.Paths[0].Path,
			Service: ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.String(),
		},
	}
	mi.Kind = v1beta1.ResourceKind
	mi.APIVersion = v1beta1.ApiGroupAndVersion
	return mi, nil
}

//根据ns来获取Ingress列表， 并转化为 我们自己的资源 MyIngressList
func (cs *ClientStore) ListByNs(ns string) *v1beta1.MyIngressList {
	list, err := k8sconfig.Factory.Networking().V1().Ingresses().Lister().Ingresses(ns).
		List(labels.Everything())
	if err != nil {
		panic(err)
	}
	myList := &v1beta1.MyIngressList{}
	for _, ingress := range list {
		myList.Items = append(myList.Items, &v1beta1.MyIngress{
			TypeMeta:   ingress.TypeMeta,
			ObjectMeta: ingress.ObjectMeta,
			Spec: v1beta1.MyIngressSpec{
				Host:    ingress.Spec.Rules[0].Host,
				Path:    ingress.Spec.Rules[0].HTTP.Paths[0].Path,
				Service: ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.String(),
			},
		})
	}
	return myList

}
