package cache

/**
* @description
* @author chengzw
* @since 2023/7/27
* @link
 */

import (
	"fmt"
	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"myinformer/lib"
	"reflect"
)

type MyFactory struct {
	client    *kubernetes.Clientset
	informers map[reflect.Type]SharedIndexInformer
}

func (this *MyFactory) PodInformer() SharedIndexInformer {
	podLW := NewListWatchFromClient(this.client.CoreV1().RESTClient(), "pods",
		"default", fields.Everything())
	indexers := Indexers{NamespaceIndex: MetaNamespaceIndexFunc}
	informer := NewSharedIndexInformer(podLW, &v1.Pod{}, 0, indexers)
	this.informers[reflect.TypeOf(&v1.Pod{})] = informer
	return informer
}
func NewMyFactory(client *kubernetes.Clientset) *MyFactory {
	return &MyFactory{client: client, informers: make(map[reflect.Type]SharedIndexInformer)}
}

func (this *MyFactory) Start() {
	ch := wait.NeverStop
	for _, i := range this.informers {
		go func(informer SharedIndexInformer) {
			informer.Run(ch)
		}(i)
	}
}

func TestSharedInformerFactory() {
	client := lib.InitClient() // clientset

	fact := NewMyFactory(client)
	fact.PodInformer().
		AddEventHandler(ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				fmt.Println(obj.(*v1.Pod).Name)
			},
		})
	fact.Start()

	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, fact.PodInformer().GetIndexer().List())
	})
	r.Run(":8080")
}
