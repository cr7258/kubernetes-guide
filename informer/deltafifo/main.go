package main

import (
	"fmt"
	"k8s.io/client-go/tools/cache"
)

/**
* @description DeltaFIFO 创建，新增和修改事件
* @author chengzw
* @since 2023/7/27
* @link
 */

type pod struct {
	Name  string
	Value float64
}

func newPod(name string, v float64) pod {
	return pod{
		Name:  name,
		Value: v,
	}
}

func podKeyFunc(obj interface{}) (string, error) {
	return obj.(pod).Name, nil
}

func main() {
	df := cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{KeyFunction: podKeyFunc})
	pod1 := newPod("pod1", 1)
	pod2 := newPod("pod2", 2)
	pod3 := newPod("pod3", 3)
	df.Add(pod1)
	df.Add(pod2)
	df.Add(pod3)
	pod1.Value = 1.1
	df.Update(pod1)
	df.Delete(pod1)

	df.Pop(func(obj interface{}) error {
		for _, delta := range obj.(cache.Deltas) {
			fmt.Println(delta.Type, delta.Object.(pod).Name, " value:", delta.Object.(pod).Value)
			switch delta.Type {
			case cache.Added:
				fmt.Println("执行新增回调")
			case cache.Updated:
				fmt.Println("执行修改回调")
			case cache.Deleted:
				fmt.Println("执行删除回调")
			}
		}
		return nil
	})
}
