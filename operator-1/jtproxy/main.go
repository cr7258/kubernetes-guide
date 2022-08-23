package main

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"github.com/yeqown/log"
	"jtproxy/pkg/filters"
	"jtproxy/pkg/sysinit"
)

func ProxyHandler(ctx *fasthttp.RequestCtx) {
	//代表匹配到了 path
	if getProxy := sysinit.GetRoute(ctx.Request); getProxy != nil {
		filters.ProxyFilters(getProxy.Filters).Do(ctx) //过滤
		getProxy.Proxy.ServeHTTP(ctx)                  //反代
	} else {
		ctx.Response.SetStatusCode(404)
		ctx.Response.SetBodyString("404...")
	}

}

//var jtthink=proxy.NewReverseProxy("www.jtthink.com",)
func main() {
	sysinit.InitConfig()
	log.Fatal(fasthttp.ListenAndServe(fmt.Sprintf(":%d", sysinit.SysConfig.Server.Port), ProxyHandler))
}
