package main

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"github.com/yeqown/fasthttp-reverse-proxy/v2"
	"jtproxy/pkg/sysinit"
	"log"
)

func ProxyHandler(ctx *fasthttp.RequestCtx) {
	//代表匹配到了 path
	if getProxy := sysinit.GetRoute(ctx.Request); getProxy != nil {
		getProxy.ServeHTTP(ctx)
	} else {
		ctx.Response.SetStatusCode(404)
		ctx.Response.SetBodyString("404...")
	}
}

var jtthink = proxy.NewReverseProxy("www.jtthink.com")

func main() {
	sysinit.InitConfig()
	log.Fatal(fasthttp.ListenAndServe(fmt.Sprintf(":%d", sysinit.SysConfig.Server.Port), ProxyHandler))
}
