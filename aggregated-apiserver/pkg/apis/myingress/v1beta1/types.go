package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// MyIngress
type MyIngress struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MyIngressSpec `json:"spec,omitempty"`
}

func (*MyIngress) New() runtime.Object {
	return &MyIngress{}
}

// +k8s:openapi-gen=true
type MyIngressSpec struct {
	Host string `json:"host,omitempty"`
	Path string `json:"path,omitempty"` // 默认是 /

	Service string `json:"service,omitempty"` //格式是 xxx:8080  默认是80
}

func (MyIngressSpec) OpenAPIDefinition() common.OpenAPIDefinition {
	var minLength int64 = 2 // host 属性至少 2 个字符
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Required: []string{"host", "path", "service"},
				Type:     []string{"object"},
				Properties: map[string]spec.Schema{
					"host": {
						SchemaProps: spec.SchemaProps{
							Type:      []string{"string"},
							Format:    "",
							MinLength: &minLength,
						},
					},
					"path": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"string"},
							Format: "",
						},
					},
					"service": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"string"},
							Format: "",
						},
					},
				},
			},
		},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// MyIngressList
type MyIngressList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MyIngress `json:"items"` //这里不能使用指针
}

func NewMyIngressList() MyIngressList {

	list := MyIngressList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Version: SchemeGroupVersion.Version,
		Group:   SchemeGroupVersion.Group,
		Kind:    "MyIngressList",
	})
	return list
}
