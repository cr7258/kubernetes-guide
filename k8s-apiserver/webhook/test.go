package main

import (
	"context"
	"fmt"
	"k8s-webhook/helper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
// TODO 专注golang、k8s云原生、Rust技术栈
func main() {
	clientset := helper.K8sClient()

	defaultSa := &corev1.ServiceAccount{}
	defaultSa.Name = "default"
	_, err := clientset.CoreV1().ServiceAccounts("default").Create(context.TODO(), defaultSa, metav1.CreateOptions{})

	ngxPod := &corev1.Pod{}
	ngxPod.Name = "nginxpod"
	ngxPod.Spec.Containers = []corev1.Container{
		{Name: "nginx", Image: "nginx"},
	}

	_, err = clientset.CoreV1().Pods("default").Create(context.TODO(), ngxPod, metav1.CreateOptions{})

	// 获取所有的Pod资源，并打印它们的名称
	pods, err := clientset.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, pod := range pods.Items {
		fmt.Printf("Pod: %s\n", pod.GetName())
	}
}
