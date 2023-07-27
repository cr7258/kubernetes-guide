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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"myinformer/lib"
	"time"
)

type PodHandler struct {
	msg string
}

func (p PodHandler) OnAdd(obj interface{}) {
	fmt.Println("OnAdd:"+p.msg, obj.(metav1.Object).GetName())
}
func (p PodHandler) OnUpdate(oldObj, newObj interface{}) {
	fmt.Println("OnUpdate" + p.msg)
}
func (p PodHandler) OnDelete(obj interface{}) {
	fmt.Println("OnDelete" + p.msg)
}

type MySharedInformer struct {
	processor *sharedProcessor
	reflector *Reflector
	fifo      *DeltaFIFO
	store     Store
}

func NewMySharedInformer(lw *ListWatch, objType runtime.Object, indexer Indexer) *MySharedInformer {
	fifo := NewDeltaFIFOWithOptions(DeltaFIFOOptions{
		KeyFunction:  MetaNamespaceKeyFunc,
		KnownObjects: indexer,
	})
	reflector := NewReflector(lw, objType, fifo, 0)
	return &MySharedInformer{processor: &sharedProcessor{}, reflector: reflector,
		fifo: fifo, store: indexer}
}

func (msi *MySharedInformer) addEventHandler(handler ResourceEventHandler) {
	lis := newProcessListener(handler, 0, 0, time.Now(),
		initialBufferSize)
	msi.processor.addListener(lis)
}

func (msi *MySharedInformer) start(ch <-chan struct{}) {
	go func() {
		for {
			msi.fifo.Pop(func(obj interface{}) error {
				for _, delta := range obj.(Deltas) {
					switch delta.Type {
					case Sync, Added:
						msi.store.Add(delta.Object)
						msi.processor.distribute(addNotification{newObj: delta.Object}, false)
					case Updated:
						if old, exists, err := msi.store.Get(delta.Object); err == nil && exists {
							msi.store.Update(delta.Object)
							msi.processor.distribute(updateNotification{newObj: delta.Object, oldObj: old}, false)
						}
					case Deleted:
						msi.store.Delete(delta.Object)
						msi.processor.distribute(deleteNotification{oldObj: delta.Object}, false)
					}
				}
				return nil
			})
		}
	}()
	go func() {
		msi.reflector.Run(ch)
	}()
	msi.processor.run(ch)
}

// 构建索引，参考 MetaNamespaceIndexFunc
func LabelsIndexFunc(obj interface{}) ([]string, error) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return []string{""}, fmt.Errorf("object has no meta: %v", err)
	}
	return []string{meta.GetLabels()["app"]}, nil
}

func TestIndexerRun() {

	indexers := Indexers{"app": LabelsIndexFunc}
	myindex := NewIndexer(DeletionHandlingMetaNamespaceKeyFunc, indexers)

	go func() {
		r := gin.New()
		r.GET("/", func(c *gin.Context) {
			// 随意找一个存在的 Pod
			ret, _ := myindex.IndexKeys("app", "robusta-forwarder")
			c.JSON(200, ret)
		})
		r.Run(":8080")
	}()

	client := lib.InitClient()
	podLW := NewListWatchFromClient(client.CoreV1().RESTClient(), "pods", "default", fields.Everything())
	msi := NewMySharedInformer(podLW, &v1.Pod{}, myindex)
	msi.addEventHandler(&PodHandler{})
	msi.start(wait.NeverStop)
}
