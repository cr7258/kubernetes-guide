package main

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	cfg, err := config.GetConfig()
	if err != nil {
		panic(err.Error())
	}

	c, err := client.New(cfg, client.Options{})
	if err != nil {
		panic(err.Error())
	}

	namespace := "default"
	podName := "wiremock-84d49c989c-vmcc9"

	// 根据 namespace 和 name 获取指定 Pod
	pod := &corev1.Pod{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: podName, Namespace: namespace}, pod)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Pod: %s\n", pod.Name)

	// 根据 namespace 和 label 获取指定 PodList
	pods := &v1.PodList{}

	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"name": "wiremock",
		},
	})

	err = c.List(context.Background(), pods, &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	})

	if err != nil {
		panic(err.Error())
	}

	for _, pod := range pods.Items {
		fmt.Printf("Pod: %s\n", pod.Name)
	}
}
