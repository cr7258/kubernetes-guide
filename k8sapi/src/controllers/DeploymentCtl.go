package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/shenyisyn/goft-gin/goft"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type DeploymentCtl struct {
	K8sClient *kubernetes.Clientset `inject:"-"`
}

func NewDeploymentCtl() *DeploymentCtl {
	return &DeploymentCtl{}
}

func (this *DeploymentCtl) GetList(c *gin.Context) goft.Json {
	list, err := this.K8sClient.AppsV1().Deployments("default").List(c, metav1.ListOptions{})
	goft.Error(err)
	return list
}

func (this *DeploymentCtl) Build(goft *goft.Goft) {
	goft.Handle("GET", "/deployments", this.GetList)
}

func (*DeploymentCtl) Name() string {
	return "DeploymentCtl"
}
