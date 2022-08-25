package k8sconfig

import (
	"context"
	"io/ioutil"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
)

//全局变量
var ServiceIP string

const JtProxySvcName = "jtproxy-svc" //默认的固定的 Svc名称
const NSFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

//用来获取 svc的ip
func init() {
	if os.Getenv("release") != "0" { //本机情况
		ServiceIP = "127.0.0.1" //写着玩的。
		return
	}
	config := K8sRestConfig()
	//pod里会有这样一个文件
	//  namespace 在这里 /var/run/secrets/kubernetes.io/serviceaccount/namespace
	ns, _ := ioutil.ReadFile(NSFile)               //取到了 命名空间
	client, err := kubernetes.NewForConfig(config) //clientset
	if err != nil {
		log.Fatal(err)
	}
	svc, err := client.CoreV1().
		Services(string(ns)).
		Get(context.Background(), JtProxySvcName, v1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}
	ServiceIP = svc.Spec.ClusterIP // 获取到控制器svc对应的clusterip

}

//POD里  体内
func K8sRestConfigInPod() *rest.Config {

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err)
	}
	return config
}

// 获取 config对象
func K8sRestConfig() *rest.Config {
	if os.Getenv("release") == "0" { //自定义环境
		log.Println("run in cluster")
		return K8sRestConfigInPod()
	}
	log.Println("run outside cluster")
	config, err := clientcmd.BuildConfigFromFlags("", "./resources/config")
	if err != nil {
		log.Fatal(err)
	}
	//config.Insecure = true
	return config
}
