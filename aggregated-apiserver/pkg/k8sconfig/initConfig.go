package k8sconfig

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
)

//全局变量

const NSFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

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
	if os.Getenv("release") == "1" { //自定义环境
		log.Println("run in cluster")
		return K8sRestConfigInPod()
	}
	log.Println("run outside cluster")
	config, err := clientcmd.BuildConfigFromFlags("", "./resources/config")
	if err != nil {
		log.Fatal(err)
	}
	//config.Insecure=true
	return config
}

// 判断所给路径文件/文件夹是否存在
func Exists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

//初始化client-go客户端
func InitClient() *kubernetes.Clientset {
	var c *kubernetes.Clientset
	var err error
	if Exists("./resources/config") {
		c, err = kubernetes.NewForConfig(K8sRestConfig())
	} else {
		c, err = kubernetes.NewForConfig(K8sRestConfigInPod())
	}
	c.RESTClient().GetRateLimiter()
	if err != nil {
		log.Fatal(err)
	}
	return c
}

var Factory informers.SharedInformerFactory

func K8sInitInformer() {
	Factory = informers.NewSharedInformerFactory(InitClient(), 0)
	IngressInformer := Factory.Networking().V1().Ingresses() //监听Ingress
	// 暂时不写自己的 回调
	IngressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	stopCh := make(chan struct{})
	Factory.Start(stopCh)
	Factory.WaitForCacheSync(stopCh)
}
