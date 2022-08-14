package lib

import (
	"fmt"
	"k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func pathContainer() []byte {
	str := `[
   {
		"op" : "replace" ,
		"path" : "/spec/containers/0/image" ,
		"value" : "nginx:1.19-alpine"
	},
    {
		"op" : "add" ,
		"path" : "/spec/initContainers" ,
		"value" : [{
			"name" : "myinit",
			"image" : "busybox:1.28",
 			"command" : ["sh", "-c", "echo The app is running!"]
 		}]
	}
]`
	return []byte(str)
}

func AdmitPods(ar v1.AdmissionReview) *v1.AdmissionResponse {
	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	if ar.Request.Resource != podResource {
		err := fmt.Errorf("expect resource to be %s", podResource)
		klog.Error(err)
		return ToV1AdmissionResponse(err)
	}

	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}
	deserializer := Codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		klog.Error(err)
		return ToV1AdmissionResponse(err)
	}

	reviewResponse := v1.AdmissionResponse{}

	if pod.Name == "shenyi" {
		reviewResponse.Allowed = false
		reviewResponse.Result = &metav1.Status{Code: 503, Message: "pod name cannot be shenyi"}
	} else {
		reviewResponse.Allowed = true
		// 修改容器镜像，模拟 Istio 注入容器
		reviewResponse.Patch = pathContainer()
		pt := v1.PatchTypeJSONPatch
		reviewResponse.PatchType = &pt
	}

	return &reviewResponse
}
