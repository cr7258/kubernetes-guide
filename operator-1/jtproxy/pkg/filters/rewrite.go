package filters

import (
	"github.com/valyala/fasthttp"
	"log"
	"regexp"
)

const RewriteAnnotation = AnnotationPrefix + "/rewrite-target"

func init() {
	registerFilter(RewriteAnnotation, (*RewriteFilter)(nil))
}

type RewriteFilter struct {
	pathValue string
	target    string //注解  值
	path      string
}

func (this *RewriteFilter) SetPath(value string) {
	this.pathValue = value
}

//可变参数。第1个是 rewrie-target:的值 如 /$1
func (this *RewriteFilter) SetValue(values ...string) {
	this.target = values[0]
}
func (this *RewriteFilter) Do(ctx *fasthttp.RequestCtx) {
	getUrl := string(ctx.RequestURI()) //获取 请求PATH  譬如  /jtthink/users
	reg, err := regexp.Compile(this.pathValue)
	if err != nil {
		log.Println(err)
		return
	}

	getUrl = reg.ReplaceAllString(getUrl, this.target)
	ctx.Request.SetRequestURI(getUrl)
	if err != nil {
		log.Println(err)
		return
	}

}
