package cache

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
)
//临时放的一个 空的 handler
type DeployHandler struct {
}
func(this *DeployHandler) OnAdd(obj interface{})               {}
func(this *DeployHandler) OnUpdate(oldObj, newObj interface{}) {}
func(this *DeployHandler) OnDelete(obj interface{})            {}

type PodHandler struct {
}
func(this *PodHandler) OnAdd(obj interface{})               {}
func(this *PodHandler) OnUpdate(oldObj, newObj interface{}) {}
func(this *PodHandler) OnDelete(obj interface{})            {}


var  Client= InitClient() //这是 clientset
var  RestConfig *rest.Config
var Factory informers.SharedInformerFactory
//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
var CfgFlags *genericclioptions.ConfigFlags
//上节课代码 做了。封装
func InitClient() *kubernetes.Clientset{
	CfgFlags =genericclioptions.NewConfigFlags(true)
	config,err:= CfgFlags.ToRawKubeConfigLoader().ClientConfig()
	if err!=nil{log.Fatalln(err)}
	c,err:=kubernetes.NewForConfig(config)

	if err!=nil{log.Fatalln(err)}
	RestConfig=config//设置了 config 。后面要用到
	return c
}

//初始化Informer 监听
func InitCache() {
	Factory =informers.NewSharedInformerFactory(Client,0)
    Factory.Apps().V1().Deployments().Informer().AddEventHandler(&DeployHandler{})

	Factory.Apps().V1().ReplicaSets().Informer().AddEventHandler(&PodHandler{})//为了偷懒
	Factory.Core().V1().Pods().Informer().AddEventHandler(&PodHandler{}) //为了偷懒
	Factory.Core().V1().Events().Informer().AddEventHandler(&PodHandler{})//偷懒
	ch:=make(chan struct{})
	Factory.Start(ch)
	Factory.WaitForCacheSync(ch)
}