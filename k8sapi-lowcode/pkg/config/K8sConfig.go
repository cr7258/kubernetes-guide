package config

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"log"
)

//全局变量
var K8sInformerFactory informers.SharedInformerFactory

type K8sConfig struct {
}

//直接初始化
func NewK8sConfig() *K8sConfig {
	cfg := &K8sConfig{}
	K8sInformerFactory = cfg.initWatch()
	stopCh := make(chan struct{})
	K8sInformerFactory.Start(stopCh)
	K8sInformerFactory.WaitForCacheSync(stopCh)
	return cfg
}
func (*K8sConfig) K8sRestConfig() *rest.Config {
	config, err := clientcmd.BuildConfigFromFlags("", "./resources/config")
	if err != nil {
		log.Fatal(err)
	}
	return config
}

//初始化client-go客户端
func (cfg *K8sConfig) InitClient() *kubernetes.Clientset {
	c, err := kubernetes.NewForConfig(cfg.K8sRestConfig())
	if err != nil {
		log.Fatal(err)
	}
	return c
}
func (cfg *K8sConfig) initWatch() informers.SharedInformerFactory {
	fact := informers.NewSharedInformerFactory(cfg.InitClient(), 0)
	fact.Core().V1().Namespaces().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	return fact
}
