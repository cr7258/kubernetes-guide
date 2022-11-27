package webui

import (
	"depplugin/pkg/webui/steps"
	"github.com/gin-gonic/gin"
	"github.com/shenyisyn/goft-gin/goft"
)

type WebCtl struct {

}

func NewWebCtl() *WebCtl {
	return &WebCtl{}
}

func(this *WebCtl) WebIndex(c *gin.Context) (v goft.Void)  {
	 config:=map[string]interface{}{
	 	"Step":steps.StepsData.GetStep(),
	 }
	 c.HTML(200,"index.html",config)
	 return
}
func(this *WebCtl) Name () string   {
	return "WebCtl"
}
func(this *WebCtl) Build(goft *goft.Goft){
	goft.Handle("GET","/",this.WebIndex)
}
