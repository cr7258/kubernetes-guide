package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MyIngress
type MyIngress struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MyIngressSpec `json:"spec,omitempty"`
}
func(*MyIngress) New() runtime.Object{
	return &MyIngress{}
}
func(*MyIngress) NamespaceScoped () bool {
	 return true
}
type MyIngressSpec struct {
	 Host string  `json:"host,omitempty"`
	 Path string  `json:"path,omitempty"`  // 默认是 /
	 Service string `json:"service,omitempty"`  //格式是 xxx:8080  默认是80
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MyIngressList
type MyIngressList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []*MyIngress `json:"items"`
}
func NewMyIngressList() *MyIngressList {
	list:= &MyIngressList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Version: SchemeGroupVersion.Version,
		Group: SchemeGroupVersion.Group,
		Kind: "MyIngressList",
	})
	return list

}


