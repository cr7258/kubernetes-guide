package builders

import (
	"github.com/shenyisyn/aapi/pkg/apis/myingress/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ApiResourceList() metav1.APIResourceList{
	apiList:=metav1.APIResourceList{
		 GroupVersion: v1beta1.SchemeGroupVersion.String(),
		 APIResources: []metav1.APIResource{
		 	{
		 		Name:"myingresses",
		 		SingularName: "myingress",
		 		Kind: "MyIngress",
		 		ShortNames: []string{"mi"},
		 		Namespaced: true,
		 		Verbs: []string{"get","list","create","watch"},
			},
		 },
	}
	apiList.APIVersion="v1"
	apiList.Kind="APIResourceList"
	return apiList
}