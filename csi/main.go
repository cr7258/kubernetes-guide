package main

import (
	"flag"
	"k8s.io/klog"
	"mycsi/pkg/bootstrap"
)

var (
	nodeID string //这个外部传进来
)

func main() {
	flag.StringVar(&nodeID, "nodeid", "", "--nodeid=xxx")
	klog.InitFlags(nil)
	flag.Parse()

	driver := bootstrap.NewMyDriver(nodeID)
	driver.Start()
}
