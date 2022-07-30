package ctrls

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/pkg/strings"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/shenyisyn/goft-gin/goft"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8sapi-lowcode/pkg/utils"
)

func convertGvr(gvr string) schema.GroupVersionResource {
	gvrList := strings.Split(gvr, "_")
	if len(gvrList) != 3 {
		panic("error gvr")
	}
	return schema.GroupVersionResource{
		Group: gvrList[0], Version: gvrList[1], Resource: gvrList[2],
	}

}

//通用表单处理
type FormCtl struct {
}

// 通用提交
func (f *FormCtl) CommonPost(c *gin.Context) goft.Json {
	var m map[string]interface{}
	goft.Error(c.ShouldBindJSON(&m))
	cc := cuecontext.New()
	cv := cc.CompileBytes(utils.MustLoadFile("./pkg/cues/fast/nginx.cue"))
	formData := cc.CompileString(m["formData"].(string))
	cv = cv.FillPath(cue.ParsePath("input"), formData)
	b, err := cv.LookupPath(cue.ParsePath("output")).MarshalJSON()
	goft.Error(err)
	fmt.Println(string(b))
	return gin.H{
		"code": 20000,
		"data": "ok",
	}
}
func (f *FormCtl) Name() string {
	return "FastController"
}
func (f *FormCtl) Build(goft *goft.Goft) {
	goft.Handle("POST", "/form/post", f.CommonPost)
}
