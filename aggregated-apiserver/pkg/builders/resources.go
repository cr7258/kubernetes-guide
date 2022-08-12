package builders

import (
	"context"
	"github.com/shenyisyn/aapi/pkg/apis/myingress/v1beta1"
	"github.com/shenyisyn/aapi/pkg/k8sconfig"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
	"strings"
)

func CreateIngress(mi *v1beta1.MyIngress) error {
	svc := mi.Spec.Service
	port := 80 //默认端口
	pathType := v1.PathTypePrefix
	svcPort := strings.Split(mi.Spec.Service, ":")
	if len(svcPort) == 2 {
		svc = svcPort[0]
		getPort, err := strconv.Atoi(svcPort[1])
		if err != nil {
			return err
		}
		port = getPort
	}

	ingress := &v1.Ingress{ObjectMeta: metav1.ObjectMeta{
		Name:      mi.Name,
		Namespace: mi.Namespace,
		Annotations: map[string]string{
			"controlBy": "myingress", //自己加了一个注解进去  ，内容 开心就好
		},
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
	}
	_, err := k8sconfig.InitClient().NetworkingV1().Ingresses(mi.Namespace).
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
