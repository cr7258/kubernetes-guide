package mycore

import (
	"context"
	"errors"
	"strings"

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

const RemoteRuntimeAddress = "172.19.0.2" // 修改远程的 runtime 地址
func initRuntimeClient() runtimeapi.RuntimeServiceClient {
	gopts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	conn, err := grpc.DialContext(ctx, RemoteRuntimeAddress+":8989", gopts...)
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
		ContainerId: "afbf835393c303028577a67cc047f10b7d39c64d1161d6d372ce5c3c772dd2e2", // 修改要连接的容器 ID
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
	resp.Url = strings.Replace(resp.Url, "[::]", RemoteRuntimeAddress, -1)
	klog.Info("修改过后的URL是：", resp.Url)
	return url.Parse(resp.Url)
}
