package main

import (
	"fmt"
	"k8s.io/kubernetes/mykubelet/mylib"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"log"
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
	pods, _ := cr.GetPods(true)
	for _, pod := range pods {
		fmt.Print(pod.Name, "容器有:")
		for _, c := range pod.Containers {
			fmt.Print(c.Name, ",")
		}
		fmt.Println()
	}
}
