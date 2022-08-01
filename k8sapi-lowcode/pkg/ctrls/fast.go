package ctrls

import (
	"github.com/gin-gonic/gin"
	"github.com/shenyisyn/goft-gin/goft"
	"k8sapi-lowcode/pkg/config"
	"k8sapi-lowcode/pkg/constances"
	"k8sapi-lowcode/pkg/utils"
)

type Fast struct {
}

func (f *Fast) NginxPreCreate(c *gin.Context) goft.Json {
	//上节课代码做了封装
	return utils.PareseCue(constances.FastNginxCue_File, constances.FastNginxCue_Param,
		utils.WithNameSpaceInject(config.K8sInformerFactory, "input", "namespace"))
}
func (f *Fast) Name() string {

	return "FastController"
}
func (f *Fast) Build(goft *goft.Goft) {
	goft.Handle("GET", "/fast/nginx/precreate", f.NginxPreCreate)
}
