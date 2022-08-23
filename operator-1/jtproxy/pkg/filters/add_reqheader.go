package filters

import (
	"github.com/valyala/fasthttp"
	"strings"
)

const AddRequestHeaderAnnotation = AnnotationPrefix + "/add-request-header"

func init() {
	registerFilter(AddRequestHeaderAnnotation, (*AddRequestHeaderFilter)(nil))
}

type AddRequestHeaderFilter struct {
	pathValue string
	target    string //注解  值
	path      string
}

func (this *AddRequestHeaderFilter) SetPath(value string) {}

//可变参数。第1个是 注解值:的值 如 /$1
func (this *AddRequestHeaderFilter) SetValue(values ...string) {
	this.target = values[0]
}
func (this *AddRequestHeaderFilter) Do(ctx *fasthttp.RequestCtx) {
	kvList := strings.Split(this.target, ";")
	for _, kv := range kvList {
		k_v := strings.Split(kv, "=")
		if len(k_v) == 2 {
			ctx.Request.Header.Add(k_v[0], k_v[1])
		}
	}

}
