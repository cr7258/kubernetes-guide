package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubernetes/pkg/kubelet/configmap"
	kubepod "k8s.io/kubernetes/pkg/kubelet/pod"
	"k8s.io/kubernetes/pkg/kubelet/secret"
	"log"
	"reflect"
)

/**
* @description 手工调用 PodManager，创建一个假的静态 Pod
* @author chengzw
* @since 2023/5/25
* @link
 */

func main() {
	restConfig, err := clientcmd.BuildConfigFromFlags("", homedir.HomeDir()+"/.kube/config")
	if err != nil {
		log.Fatalln(err)
	}
	client, err := clientset.NewForConfig(restConfig)
	if err != nil {
		log.Fatalln(err)
	}

	ch := make(chan struct{})
	fact := informers.NewSharedInformerFactory(client, 0)
	fact.Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	fact.Start(ch)

	secretManager := secret.NewSimpleSecretManager(client)
	configMapManager := configmap.NewSimpleConfigMapManager(client)

	// 等待 Node 缓存同步完成
	if waitMap := fact.WaitForCacheSync(ch); waitMap[reflect.TypeOf(&v1.Node{})] {
		nodeLister := fact.Core().V1().Nodes().Lister()
		mirrorPodClient := kubepod.NewBasicMirrorClient(client, "myjtthink", nodeLister)
		podManager := kubepod.NewBasicPodManager(mirrorPodClient, secretManager, configMapManager)

		pod := &v1.Pod{}
		pod.Name = "kube-mystatic-myjtthink"
		pod.Namespace = "default"
		pod.Spec.NodeName = "myjtthink"
		pod.Spec.Containers = []v1.Container{
			{
				Name:  "mystatic-container",
				Image: "static:v1",
			},
		}
		err := podManager.CreateMirrorPod(pod)
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Println("静态 Pod 创建成功")
	}
}
