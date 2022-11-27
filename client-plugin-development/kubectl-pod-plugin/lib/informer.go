package lib

import (
	"k8s.io/client-go/informers"
)
type PodHandler struct {
}
func(this *PodHandler) OnAdd(obj interface{})               {}
func(this *PodHandler) OnUpdate(oldObj, newObj interface{}) {}
func(this *PodHandler) OnDelete(obj interface{})            {}

var fact informers.SharedInformerFactory
func InitCache() {
	fact =informers.NewSharedInformerFactory(client,0)
	fact.Core().V1().Pods().Informer().AddEventHandler(&PodHandler{})
	fact.Core().V1().Events().Informer().AddEventHandler(&PodHandler{}) //为了偷懒
	ch:=make(chan struct{})
	fact.Start(ch)
	fact.WaitForCacheSync(ch)
}

