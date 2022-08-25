package filters

import (
	"github.com/valyala/fasthttp"
	"strings"
)
const AddReponseHeaderAnnotation=AnnotationPrefix+"/add-response-header"

func init() {
	//注册响应 过滤器
	registerReponseFilter(AddReponseHeaderAnnotation,(*AddResponseHeaderFilter)(nil) )
}
type AddResponseHeaderFilter struct {
    pathValue string
    target string  //注解  值
    path string
}
func(this *AddResponseHeaderFilter) SetPath(value  string){}
//可变参数。第1个是 注解值:的值 如 /$1
func(this *AddResponseHeaderFilter) SetValue(values ...string){
	this.target=values[0]
}
func(this *AddResponseHeaderFilter) Do(ctx *fasthttp.RequestCtx){
	 kvList:=strings.Split(this.target,";")
	 for _,kv:=range kvList{
	 	k_v:=strings.Split(kv,"=")
	 	if len(k_v)==2{
			ctx.Response.Header.Add(k_v[0],k_v[1])
		}
	 }

}