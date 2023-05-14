package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/kubernetes/cmd/kube-apiserver/app"
	"k8s.io/kubernetes/cmd/kube-apiserver/app/options"
	authzmodes "k8s.io/kubernetes/pkg/kubeapiserver/authorizer/modes"
	"k8s.io/kubernetes/pkg/serviceaccount"
	"log"
)

var (
	// Scheme 定义了资源序列化和反序列化的方法以及资源类型和版本的对应关系
	Scheme = runtime.NewScheme()
	//  编解码器工厂
	Codecs = serializer.NewCodecFactory(Scheme)

	EtcdServers = []string{"http://127.0.0.1:2379"}

	Issuers = []string{"https://kubernetes.default.svc.cluster.local"}
)

func main() {
	s := options.NewServerRunOptions()
	// 设置默认配置 + 参数处理
	completedOptions, err := app.Complete(s)
	if err != nil {
		fmt.Println(err)
	}

	// 填充参数
	completedOptions.Etcd.StorageConfig.Transport.ServerList = EtcdServers
	completedOptions.Authentication.ServiceAccounts.Issuers = Issuers
	completedOptions.Authentication.APIAudiences = Issuers
	completedOptions.Authentication.ServiceAccounts.KeyFiles = []string{"../../certs/sa.crt"}
	completedOptions.ServiceAccountSigningKeyFile = "../../certs/sa.key"

	completedOptions.Authentication.Anonymous.Allow = true
	completedOptions.Authorization.Modes = []string{authzmodes.ModeAlwaysAllow}

	sk, err := keyutil.PrivateKeyFromFile(completedOptions.ServiceAccountSigningKeyFile)
	completedOptions.ServiceAccountIssuer, err = serviceaccount.JWTTokenGenerator(completedOptions.Authentication.ServiceAccounts.Issuers[0], sk)

	if err != nil {
		fmt.Println(err)
	}

	// 校验参数
	errs := completedOptions.Validate()
	if errs != nil && len(errs) > 0 {
		fmt.Println(errs)
	}

	ch := genericapiserver.SetupSignalHandler()
	server, err := app.CreateServerChain(completedOptions, ch)
	if err != nil {
		log.Fatalln(err)
	}

	prepared, err := server.PrepareRun()
	if err != nil {
		log.Fatalln(err)
	}
	prepared.Run(ch)
}
