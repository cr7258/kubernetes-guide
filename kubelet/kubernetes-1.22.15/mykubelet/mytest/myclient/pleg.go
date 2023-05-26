package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/clock"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/mykubelet/mylib"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/pleg"
	"log"
	"net/http"
	"time"
)

func checkerr(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func main() {
	rs := &mylib.MyRuntimeService{} // CRI 模拟实现
	// 模拟创建 kubelet 封装的 runtime
	var cr kubecontainer.Runtime = mylib.NewContianerRuntime(rs, "containerd")
	cache := kubecontainer.NewCache()
	p := pleg.NewGenericPLEG(cr, 1000, time.Second*1, cache, clock.RealClock{})
	go func() {
		for {
			select {
			case v := <-p.Watch():
				if v.Type != pleg.ContainerStarted {
					fmt.Println(v)
					break
				}
			}
		}
	}()
	p.Start()

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		mylib.MockData_Pods[0].State = runtimeapi.PodSandboxState_SANDBOX_NOTREADY
		writer.Write([]byte("Pod 状态变更"))
	})

	http.ListenAndServe(":8080", nil)
}
