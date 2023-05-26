package main

import (
	"encoding/json"
	"fmt"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubernetes/mykubelet/mycore"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
	"log"
	"net/http"
	"sort"
)

func main() {
	restConfig, err := clientcmd.BuildConfigFromFlags("", homedir.HomeDir()+"/.kube/config")
	if err != nil {
		log.Fatalln(err)
	}
	client, err := clientset.NewForConfig(restConfig)
	if err != nil {
		log.Fatalln(err)
	}

	nodeName := "kubelet-demo-control-plane"
	pc := mycore.NewPodCache(client, nodeName)

	go func() {
		fmt.Println("开启 HTTP 服务")
		// 用于显示当前缓存有多少 Pod
		http.HandleFunc("/pods", func(writer http.ResponseWriter, request *http.Request) {
			pods := []string{}
			for _, pod := range pc.PodManager.GetPods() {
				pods = append(pods, pod.Namespace+"/"+pod.Name)
			}
			sort.Strings(pods)
			b, _ := json.Marshal(pods)
			writer.Header().Add("Content-Type", "application/json")
			writer.Write(b)
		})
		http.ListenAndServe(":8080", nil)
	}()
	fmt.Println("开始监听")
	for item := range pc.PodConfig.Updates() {
		pods := item.Pods
		switch item.Op {
		case kubetypes.ADD:
			for _, pod := range pods {
				pc.PodManager.AddPod(pod)
			}
			break
		case kubetypes.UPDATE:
			for _, pod := range pods {
				pc.PodManager.UpdatePod(pod)
			}
			break
		case kubetypes.DELETE:
			for _, pod := range pods {
				pc.PodManager.DeletePod(pod)
			}
		}
	}
}
