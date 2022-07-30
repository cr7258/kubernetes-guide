package ctrls

import (
	"github.com/gin-gonic/gin"
	"github.com/shenyisyn/goft-gin/goft"
	"k8sapi-lowcode/pkg/utils"
)

type Fast struct {
}

const (
	FastNginxCue_File  = "./pkg/cues/fast/nginx.cue"
	FastNginxCue_Param = "input"
)

func (f *Fast) NginxPreCreate(c *gin.Context) goft.Json {
	//上节课代码做了封装
	return utils.PareseCue(FastNginxCue_File, FastNginxCue_Param,
		utils.WithNameSpaceInject("input", "namespace"))
}
func (f *Fast) Name() string {
	return "FastController"
}
func (f *Fast) Build(goft *goft.Goft) {
	goft.Handle("GET", "/fast/nginx/precreate", f.NginxPreCreate)
}
