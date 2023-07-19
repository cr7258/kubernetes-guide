package main

import (
	"operator-3/pkg/builder"
	"operator-3/pkg/k8sconfig"
)

func main() {
	builder.InitImageCache(100) //很重要，要加，初始化 镜像缓存，默认100个
	k8sconfig.InitManager()
}
