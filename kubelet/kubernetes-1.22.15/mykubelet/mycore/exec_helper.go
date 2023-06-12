package mycore

import (
	"context"
	"errors"

	"github.com/emicklei/go-restful"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"k8s.io/apimachinery/pkg/util/proxy"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/cri/streaming"
	"log"
	"net/http"
	"net/url"

	"time"
)

type responder struct{}

func (r *responder) Error(w http.ResponseWriter, req *http.Request, err error) {
	klog.ErrorS(err, "Error while proxying request")
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func proxyStream(w http.ResponseWriter, r *http.Request, url *url.URL) {
	handler := proxy.NewUpgradeAwareHandler(url, nil /*transport*/, false /*wrapTransport*/, true /*upgradeRequired*/, &responder{})
	handler.ServeHTTP(w, r)
}

func GetExec(request *restful.Request, response *restful.Response) {
	//   pod.Name + "_" + pod.Namespace  == PodFullName
	url, err := GetUrl()
	if err != nil {
		streaming.WriteError(err, response.ResponseWriter)
		return
	}
	proxyStream(response.ResponseWriter, request.Request, url)
}

const RemoteRuntimeAddress = "192.168.2.150:8989" // 修改远程的 runtime 地址
func initRuntimeClient() runtimeapi.RuntimeServiceClient {
	gopts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	conn, err := grpc.DialContext(ctx, RemoteRuntimeAddress, gopts...)
	if err != nil {
		log.Fatalln(err)
	}
	return runtimeapi.NewRuntimeServiceClient(conn)

}

func runtimeExec(req *runtimeapi.ExecRequest) (*runtimeapi.ExecResponse, error) {
	runtimeClient := initRuntimeClient()
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*5)
	defer cancel()

	resp, err := runtimeClient.Exec(ctx, req)
	if err != nil {
		klog.ErrorS(err, "Exec cmd from runtime service failed", "containerID", req.ContainerId, "cmd", req.Cmd)
		return nil, err
	}
	klog.V(10).InfoS("[RemoteRuntimeService] Exec Response")

	if resp.Url == "" {
		errorMessage := "URL is not set"
		err := errors.New(errorMessage)
		klog.ErrorS(err, "Exec failed")
		return nil, err
	}

	return resp, nil

}
func GetUrl() (*url.URL, error) {
	req := &runtimeapi.ExecRequest{
		ContainerId: "my-nginx", // 修改要连接的容器 ID
		Cmd:         []string{"ls"},
		Tty:         false,
		Stdin:       true,
		Stdout:      true,
		Stderr:      true,
	}
	resp, err := runtimeExec(req)
	if err != nil {
		return nil, err
	}
	klog.Info("得到的URL是：", resp.Url)
	return url.Parse(resp.Url)
}
