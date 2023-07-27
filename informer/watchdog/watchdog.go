package watchdog

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

/**
* @description 实现简单的 Informer
* @author chengzw
* @since 2023/7/27
* @link
 */

type Watchdog struct {
	lw      *cache.ListWatch
	objType runtime.Object
	h       cache.ResourceEventHandler

	reflector *cache.Reflector
	fifo      *cache.DeltaFIFO
	store     cache.Store
}

func NewWatchdog(lw *cache.ListWatch, objType runtime.Object, h cache.ResourceEventHandler) *Watchdog {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)
	fifo := cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{
		KeyFunction:  cache.MetaNamespaceKeyFunc,
		KnownObjects: store,
	})
	rf := cache.NewReflector(lw, objType, fifo, 0)
	return &Watchdog{
		store:     store,
		fifo:      fifo,
		lw:        lw,
		objType:   objType,
		h:         h,
		reflector: rf,
	}
}

func (wd *Watchdog) Run(ch <-chan struct{}) {
	go func() {
		wd.reflector.Run(ch)
	}()

	for {
		// 好比 informer 在不断消费 DeltaFIFO
		wd.fifo.Pop(func(obj interface{}) error {
			for _, delta := range obj.(cache.Deltas) {
				switch delta.Type {
				case cache.Sync, cache.Added:
					wd.store.Add(delta.Object)
					wd.h.OnAdd(delta.Object)
				case cache.Updated:
					if old, exist, err := wd.store.Get(delta.Object); err == nil && exist {
						wd.store.Update(delta.Object)
						wd.h.OnUpdate(old, delta.Object)
					}
				case cache.Deleted:
					wd.store.Delete(delta.Object)
					wd.h.OnDelete(delta.Object)
				}
			}
			return nil
		})
	}
}
