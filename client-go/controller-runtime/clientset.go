package main

import (
	"context"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	homedir := homedir.HomeDir()
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(homedir, ".kube", "config"))
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	namespace := "default"
	podName := "wiremock-84d49c989c-vmcc9"
	labelSelector := "name=wiremock"

	// 根据 namespace 和 name 获取指定 Pod
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Pod: %s\n", pod.Name)

	// 根据 namespace 和 label 获取指定 PodList
	podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		panic(err.Error())
	}

	for _, pod := range podList.Items {
		fmt.Printf("Pod Name: %s\n", pod.Name)
	}
}
