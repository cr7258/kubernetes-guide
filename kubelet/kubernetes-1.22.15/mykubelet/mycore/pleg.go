package mycore

import (
	"k8s.io/apimachinery/pkg/util/clock"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/cri/remote"
	"k8s.io/kubernetes/pkg/kubelet/pleg"
	"log"
	"time"
)

const (
	ContainerdAddress = "192.168.2.150:8989"
)

// 临时代码，启动一个 虚拟的 pleg
// 其中 click和cache是要 传进来的，否则不共用
func StartPleg(ck clock.RealClock, cache kubecontainer.Cache) {
	rs, err := remote.NewRemoteRuntimeService(ContainerdAddress, time.Second*3)
	if err != nil {
		log.Fatalln(err)
	}

	cr := NewContainerRuntime(rs)

	// 每隔 1s 读取 Containerd 的 CRI 接口进行比对
	p := pleg.NewGenericPLEG(cr, 1000, time.Second*1, cache, ck)
	p.Start()
}
