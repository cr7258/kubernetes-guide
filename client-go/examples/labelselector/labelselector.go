package main

import (
	"context"
	"examples/util"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/**
* @description 根据标签选择资源
* @author chengzw
* @since 2023/8/15
* @link
 */

func main() {
	clientset, config := util.InitClient()
	// 方式一 clientset
	podList, _ := clientset.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app=nginx",
	})
	for _, pod := range podList.Items {
		println(pod.Name)
	}

	fmt.Println("========================================")
	// 方式二 controller-runtime
	labelSelector, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "nginx",
		},
	})
	c, _ := client.New(config, client.Options{})
	podList2 := &v1.PodList{}
	_ = c.List(context.Background(), podList2, &client.ListOptions{
		Namespace:     "default",
		LabelSelector: labelSelector,
	})
	for _, pod := range podList2.Items {
		println(pod.Name)
	}
}
