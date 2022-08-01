package ctrls

import (
	"cuelang.org/go/cue"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/shenyisyn/goft-gin/goft"
	"k8sapi-lowcode/pkg/config"
	"k8sapi-lowcode/pkg/store"
	"k8sapi-lowcode/pkg/utils"
)

//通用表单处理
type FormCtl struct {
	Schemes   *config.WorkScheme `inject:"-"`
	EtcdStore *store.EtcdStore   `inject:"-"`
}

// 通用提交
func (f *FormCtl) CommonPost(c *gin.Context) goft.Json {
	var m map[string]interface{}
	goft.Error(c.ShouldBindJSON(&m))

	gvr := utils.ConvertToGvr(m["gvr"].(string))
	formString := m["formData"].(string) //表单数据 --- 注意 是字符串

	//从注册 的 scheme集合中取出 对应的cue.Value
	cv, err := f.Schemes.GetScheme(gvr)
	goft.Error(err)

	formData := cv.Context().CompileString(formString)
	fmt.Println(formData)
	goft.Error(formData.Err())

	cv = cv.FillPath(cue.ParsePath("input"), formData) //fill 也可能出错，要处理
	goft.Error(cv.Err())

	goft.Error(f.EtcdStore.Add(gvr, formString))

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
