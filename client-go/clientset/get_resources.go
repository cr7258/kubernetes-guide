package main

import (
	"context"
	"flag"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

func ParseConfig(configPath string) (*kubernetes.Clientset, error) {
	var kubeconfigPath *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfigPath = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfigPath = flag.String("kubeconfig", configPath, "absolute path to the kubeconfig file")
	}
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfigPath)
	if err != nil {
		return nil, err
	}
	// 生成 clientSet
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return clientSet, err
	}
	return clientSet, nil
}

func ListCm(c *kubernetes.Clientset, ns string) error {
	configMaps, err := c.CoreV1().ConfigMaps(ns).List(context.TODO(), metav1.ListOptions{Limit: 500})
	if err != nil {
		return err
	}
	for _, cm := range configMaps.Items {
		fmt.Printf("configMap name: %v, configData: %v \n", cm.Name, cm.Data)
	}
	return nil
}

func ListNodes(c *kubernetes.Clientset) error {
	nodeList, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, node := range nodeList.Items {
		fmt.Printf("nodeName: %v, status: %v\n", node.GetName(), node.GetCreationTimestamp())
	}
	return nil
}

func ListPods(c *kubernetes.Clientset, ns string) {
	pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, v := range pods.Items {
		fmt.Printf("namespace: %v podname: %v podstatus: %v \n", v.Namespace, v.Name, v.Status.Phase)
	}
}

func ListDeployment(c *kubernetes.Clientset, ns string) error {
	deployments, err := c.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, v := range deployments.Items {
		fmt.Printf("deploymentname: %v, available: %v, ready: %v\n", v.GetName(), v.Status.AvailableReplicas, v.Status.ReadyReplicas)
	}
	return nil
}

func main() {
	var namespace = "kube-system"
	configPath := "~/.kube/config"
	config, err := ParseConfig(configPath)
	if err != nil {
		fmt.Printf("load config error: %v\n", err)
	}
	fmt.Println("list pods")
	ListPods(config, namespace)
	fmt.Println("list cm")
	if err = ListCm(config, namespace); err != nil {
		fmt.Printf("list cm error: %v", err)
	}
	fmt.Println("list nodes")
	if err = ListNodes(config); err != nil {
		fmt.Printf("list nodes error: %v", err)
	}
	fmt.Println("list deployment")
	if err = ListDeployment(config, namespace); err != nil {
		fmt.Printf("list deployment error: %v", err)
	}
}
