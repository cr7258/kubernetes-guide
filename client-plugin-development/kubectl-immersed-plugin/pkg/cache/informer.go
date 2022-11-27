package cache

import (
	"depplugin/pkg/utils"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"log"
)
//临时放的一个 空的 handler
type DeployHandler struct {
}
func(this *DeployHandler) OnAdd(obj interface{})               {
     utils.DeployChan<-obj.(*appv1.Deployment)
}
func(this *DeployHandler) OnUpdate(oldObj, newObj interface{}) {
	utils.DeployChan<-newObj.(*appv1.Deployment)  //没啥好讲的
}
func(this *DeployHandler) OnDelete(obj interface{})            {
	utils.DeployChan<-obj.(*appv1.Deployment)
}

type PodHandler struct {
}
func(this *PodHandler) OnAdd(obj interface{})               {
    utils.PodChan<-obj.(*corev1.Pod)
}
func(this *PodHandler) OnUpdate(oldObj, newObj interface{}) {
	utils.PodChan<-newObj.(*corev1.Pod)
}
func(this *PodHandler) OnDelete(obj interface{})            {
	utils.PodChan<-obj.(*corev1.Pod)
}


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
	Factory.Apps().V1().ReplicaSets().Informer().AddEventHandler(&cache.ResourceEventHandlerFuncs{})//为了偷懒
	Factory.Core().V1().Pods().Informer().AddEventHandler(&PodHandler{})
	Factory.Core().V1().Events().Informer().AddEventHandler(&cache.ResourceEventHandlerFuncs{})//偷懒
	ch:=make(chan struct{})
	Factory.Start(ch)
	Factory.WaitForCacheSync(ch)
}