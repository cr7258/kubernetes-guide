package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

/**
* @description 手工初始化 scheme
* @author chengzw
* @since 2023/7/29
* @link
 */

func main() {
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	fmt.Println(s.AllKnownTypes())
}
