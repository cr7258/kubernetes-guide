package filters

import (
	"github.com/valyala/fasthttp"
	"reflect"
)
const AnnotationPrefix="jtthink.ingress.kubernetes.io"
//所有过滤器 的接口
type ProxyFileter interface {
	SetPath(path string)  //用来设置  path的设置（带正则支持)-----并不是所有过滤器都要用到
	SetValue(values ...string)  //本方法 用来 设置
	Do(ctx *fasthttp.RequestCtx)
}
type ProxyFilters []ProxyFileter
func(this ProxyFilters) Do(ctx *fasthttp.RequestCtx){
	for _,filter:=range this {
		filter.Do(ctx)
	}
}
func(this ProxyFilters) SetPath(path string ){
	for _,filter:=range this {
		filter.SetPath(path)
	}
}

//针对Request
var FileterList=map[string]ProxyFileter{}
//针对Response
var FileterListReponse=map[string]ProxyFileter{}
//注册过滤器(request)
func registerFilter(key  string ,filter ProxyFileter)  {
	FileterList[key]= filter
}
//注册过滤器(reseponse)
func registerReponseFilter(key  string ,filter ProxyFileter)  {
	FileterListReponse[key]= filter
}
func init() {

}
//检查注解是否 和预设的 过滤器 匹配
func CheckAnnotations(annos map[string]string,isrsp bool,exts ...string  ) []ProxyFileter{
	fileters:=[]ProxyFileter{}
	var list map[string]ProxyFileter
	if isrsp{  //代表是响应过滤器
		list=FileterListReponse
	}else{
		list=FileterList
	}
	for anno_key,anno_value:=range annos{
		for filter_key,filterReflect:=range list{
			if anno_key==filter_key{
				t:=reflect.TypeOf(filterReflect)
				if t.Kind()==reflect.Ptr{
					t=t.Elem()
				}
				filter:=reflect.New(t).Interface().(ProxyFileter)
				params:=[]string{anno_value}
				params=append(params,exts...)
				filter.SetValue(params...)
				fileters=append(fileters,filter)
			}
		}
	}
	return fileters
}
