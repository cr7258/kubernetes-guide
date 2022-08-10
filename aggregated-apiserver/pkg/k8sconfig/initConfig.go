package k8sconfig

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
)
//全局变量

const NSFile="/var/run/secrets/kubernetes.io/serviceaccount/namespace"

//POD里  体内
func K8sRestConfigInPod() *rest.Config{
	config,err:=rest.InClusterConfig()
	if err!=nil{
		log.Fatal(err)
	}
	return config
}
// 获取 config对象
func K8sRestConfig() *rest.Config{
	if os.Getenv("release")=="1"{ //自定义环境
		log.Println("run in cluster")
		return K8sRestConfigInPod()
	}
	log.Println("run outside cluster")
	config, err := clientcmd.BuildConfigFromFlags("","./resources/config" )
	if err!=nil{
	   log.Fatal(err)
	}
	//config.Insecure=true
	return config
}
