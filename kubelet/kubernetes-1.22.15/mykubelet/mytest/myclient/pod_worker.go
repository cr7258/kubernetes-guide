package main

/**
* @description 手动调用 PodWorkers，理解 managePodLoop，创建虚拟 Pod
* @author chengzw
* @since 2023/5/29
* @link
 */
import (
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/mykubelet/mycore"
	"k8s.io/kubernetes/pkg/kubelet/container"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
	"log"
	"net/http"
	"sort"
	"time"
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

	nodeName := "myjtthink" // 请大家自行选择一个Ready 节点名称
	pc := mycore.NewPodCache(client, nodeName)

	go func() {
		fmt.Println("开启启动http服务")
		// 用于显示 当前缓存有多少POD
		http.HandleFunc("/pods", func(writer http.ResponseWriter, request *http.Request) {
			pods := []string{}
			for _, pod := range pc.PodManager.GetPods() {
				pods = append(pods, pod.Namespace+"/"+pod.Name)
			}
			sort.Strings(pods)
			b, _ := json.Marshal(pods)
			writer.Header().Add("Content-type", "application/json")
			writer.Write(b)
		})
		http.HandleFunc("/setcache", func(writer http.ResponseWriter, request *http.Request) {
			podid := request.URL.Query().Get("id")
			if podid == "" {
				writer.Write([]byte("缺少ID"))
				return
			}
			//随便构建一个 status
			staus := &container.PodStatus{
				ID: types.UID(podid),
				SandboxStatuses: []*v1alpha2.PodSandboxStatus{
					{
						Id:    podid,
						State: v1alpha2.PodSandboxState_SANDBOX_READY,
					},
				},
			}
			pc.InnerPodCache.Set(types.UID(podid), staus, nil, time.Now())
			writer.Write([]byte("设置成功"))
		})

		http.ListenAndServe(":8080", nil)
	}()
	fmt.Println("开始监听")

	for item := range pc.PodConfig.Updates() {
		pods := item.Pods
		switch item.Op {
		case kubetypes.ADD:
			for _, p := range pods {
				pc.PodManager.AddPod(p)                          // 加入缓存
				pc.PodWorkers.UpdatePod(mycore.UpdatePodOptions{ // 执行 dispatchWork
					UpdateType: kubetypes.SyncPodCreate,
					StartTime:  pc.Clock.Now(),
					Pod:        p,
					MirrorPod:  nil,
				})
			}
			break
		case kubetypes.UPDATE:
			break
		}
	}
}
