package builders

import (
	"context"
	"github.com/shenyisyn/aapi/pkg/apis/myingress/v1beta1"
	"github.com/shenyisyn/aapi/pkg/k8sconfig"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"strconv"
	"strings"
)

func makeIngress(mi *v1beta1.MyIngress) (*v1.Ingress, error) {
	svc := mi.Spec.Service
	port := 80 //默认端口
	pathType := v1.PathTypePrefix
	svcPort := strings.Split(mi.Spec.Service, ":")
	if len(svcPort) == 2 {
		svc = svcPort[0]
		getPort, err := strconv.Atoi(svcPort[1])
		if err != nil {
			return nil, err
		}
		port = getPort
	}
	annotations := mi.Annotations
	annotations["controlBy"] = "myingress" //自己加了一个注解进去  ，内容 开心就好
	return &v1.Ingress{ObjectMeta: metav1.ObjectMeta{
		Name:        mi.Name,
		Namespace:   mi.Namespace,
		Annotations: annotations,
	},
		Spec: v1.IngressSpec{
			Rules: []v1.IngressRule{
				{
					Host: mi.Spec.Host,
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
								{
									Path:     mi.Spec.Path,
									PathType: &pathType,
									Backend: v1.IngressBackend{
										Service: &v1.IngressServiceBackend{
											Name: svc, //服务名
											Port: v1.ServiceBackendPort{
												Number: int32(port), //端口
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}, nil

}
func PatchIngress(apply *v1beta1.MyIngress) (*v1beta1.MyIngress, error) {
	const applyKey = "kubectl.kubernetes.io/last-applied-configuration"
	miJson := apply.Annotations[applyKey]

	newMi := &v1beta1.MyIngress{}
	err := json.Unmarshal([]byte(miJson), newMi)
	if err != nil {
		return nil, err
	}
	parsedIngress, err := makeIngress(newMi)
	if err != nil {
		return nil, err
	}
	//得到原有的ingress
	oldIngress, err := k8sconfig.Factory.Networking().V1().Ingresses().Lister().
		Ingresses(newMi.Namespace).
		Get(newMi.Name)
	if err != nil {
		return nil, err
	}
	oldIngress.Annotations = parsedIngress.Annotations
	//视频中提到的 思考题
	oldIngress.Annotations[applyKey] = miJson //保存这个annotation ，否则会有警告
	oldIngress.Labels = parsedIngress.Labels
	oldIngress.Spec = parsedIngress.Spec
	_, err = k8sconfig.InitClient().NetworkingV1().Ingresses(newMi.Namespace).
		Update(context.Background(), oldIngress, metav1.UpdateOptions{})
	return newMi, err
}
func CreateIngress(mi *v1beta1.MyIngress) error {
	ingress, err := makeIngress(mi)
	if err != nil {
		return err
	}
	_, err = k8sconfig.InitClient().NetworkingV1().Ingresses(mi.Namespace).
		Create(context.Background(), ingress, metav1.CreateOptions{})
	return err
}

func ApiResourceList() metav1.APIResourceList {
	apiList := metav1.APIResourceList{
		GroupVersion: v1beta1.SchemeGroupVersion.String(),
		APIResources: []metav1.APIResource{
			{
				Name:         "myingresses",
				SingularName: "myingress",
				Kind:         "MyIngress",
				ShortNames:   []string{"mi"},
				Namespaced:   true,
				Verbs:        []string{"get", "list", "create", "watch"},
			},
		},
	}
	apiList.APIVersion = "v1"
	apiList.Kind = "APIResourceList"
	return apiList
}
