package config

import (
	"cuelang.org/go/cue"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8sapi-lowcode/pkg/constances"
	"k8sapi-lowcode/pkg/utils"
)

//注入所用
type SchemeConfig struct {
}

func NewSchemeConfig() *SchemeConfig {
	return &SchemeConfig{}
}

func (cc *SchemeConfig) InitWorkScheme() *WorkScheme {
	ws := NewWorkScheme()
	ws.AddScheme(schema.GroupVersionResource{
		Group: "k8s.jtthink.com", Version: "v1", Resource: "fastnginxs",
	}, utils.MustParseFileToCueValue(constances.FastNginxCue_File))
	return ws
}

type WorkScheme struct {
	schemes map[schema.GroupVersionResource]cue.Value
}

//初始化
func NewWorkScheme() *WorkScheme {
	return &WorkScheme{schemes: make(map[schema.GroupVersionResource]cue.Value)}
}

// 注册、加入 scheme
func (w WorkScheme) AddScheme(gvr schema.GroupVersionResource, v cue.Value) {
	if v.Err() == nil { // 防止有错也加进来
		w.schemes[gvr] = v
	}
}
func (w WorkScheme) GetScheme(gvr schema.GroupVersionResource) (cue.Value, error) {
	if v, ok := w.schemes[gvr]; ok {
		return v, nil
	}
	return cue.Value{}, fmt.Errorf("Not Found Scheme %s", gvr.String())
}
func (w WorkScheme) MustGetScheme(gvr schema.GroupVersionResource) cue.Value {
	if v, ok := w.schemes[gvr]; ok {
		return v
	}
	panic("Not Found Scheme " + gvr.String())
}
