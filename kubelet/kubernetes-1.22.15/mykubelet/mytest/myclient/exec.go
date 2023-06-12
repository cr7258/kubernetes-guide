package main

import (
	"github.com/emicklei/go-restful"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/mykubelet/mycore"
	"net/http"
)

// 启动 kubelet 模拟 exec 服务端
func main() {
	container := restful.NewContainer()
	ws := new(restful.WebService)
	ws.Path("/exec")
	{
		ws.Route(ws.GET("/{podNamespace}/{podID}/{containerName}").
			To(mycore.GetExec).
			Operation("getExec"))
		ws.Route(ws.POST("/{podNamespace}/{podID}/{containerName}").
			To(mycore.GetExec).
			Operation("getExec"))
		ws.Route(ws.GET("/{podNamespace}/{podID}/{uid}/{containerName}").
			To(mycore.GetExec).
			Operation("getExec"))
		ws.Route(ws.POST("/{podNamespace}/{podID}/{uid}/{containerName}").
			To(mycore.GetExec).
			Operation("getExec"))
	}

	container.Add(ws)

	klog.Info("启动 kubelet exec 服务，监听9090端口")
	http.ListenAndServe(":9090", container)
}
