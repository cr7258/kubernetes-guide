package config

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"log"
)

//全局变量
var K8sInformerFactory informers.SharedInformerFactory

type K8sConfig struct{}

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

//初始化 动态客户端  ++  新增的函数
func (this *K8sConfig) InitDynamicClient() dynamic.Interface {
	client, err := dynamic.NewForConfig(this.K8sRestConfig())
	if err != nil {
		log.Fatal(err)
	}
	return client
}

//获取  所有api groupresource
// 这个要 缓存起来。 不然反复从k8s api获取会比较慢
func (this *K8sConfig) RestMapper() *meta.RESTMapper {
	gr, err := restmapper.GetAPIGroupResources(this.InitClient().Discovery())
	if err != nil {
		log.Fatal(err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(gr)
	return &mapper
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
