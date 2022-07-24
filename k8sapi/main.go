package main

import (
	"github.com/shenyisyn/goft-gin/goft"
	"k8sapi/src/configs"
	"k8sapi/src/controllers"
)

func main() {
	goft.Ignite().Config(configs.NewK8sConfig()).Mount("v1", controllers.NewDeploymentCtl()).Launch()
}
