package main

/**
* @description
* @author chengzw
* @since 2023/7/29
* @link
 */
import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"my-controller-runtime/lib"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"time"
)

func main() {
	mgr, err := manager.New(lib.K8sRestConfig(),
		manager.Options{
			Logger: logf.Log.WithName("test"),
		})
	lib.Check(err)

	go func() {
		time.Sleep(time.Second * 3)
		pod := &v1.Pod{}
		err = mgr.GetClient().Get(context.TODO(),
			types.NamespacedName{Namespace: "default", Name: "wiremock-84d49c989c-8jl29"}, pod)
		lib.Check(err)
		fmt.Println(pod)
	}()
	err = mgr.Start(context.Background())
	lib.Check(err)
}
