package main

import (
	"context"
	"examples/util"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

/**
* @description Klog 日志工具使用
* @author chengzw
* @since 2023/8/24
* @link
 */

func main() {
	clientset, _ := util.InitClient()
	namespace := "default"
	name := "my-config"

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "Error when getting config map", "ConfigMap", klog.KRef(namespace, name), "Config", "myconfig")
		return
	}
	fmt.Println(cm)
}
