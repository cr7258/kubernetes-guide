package filter1

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"net/url"
)

type MyHttpContext struct {
	types.DefaultHttpContext
}

func (*MyHttpContext) OnHttpRequestHeaders(int, bool) types.Action {
	// 获取请求头，:path 是固定的值
	p, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil {
		proxywasm.LogError(err.Error())
	} else {
		base, _ := url.Parse(p)
		proxywasm.LogInfo("当前的请求参数是: " + base.RawQuery)
		// user 不是 chengzw 就返回 401 未授权
		if getUser := base.Query().Get("user"); getUser != "chengzw" {
			proxywasm.SendHttpResponse(401, [][2]string{
				{"content-type", "text/plain;charset=utf-8"},
			}, []byte("参数不正确或用户没有权限"), -1)
			return types.ActionPause
		}
		return types.ActionContinue
	}
	return types.ActionContinue
}

func (*MyHttpContext) OnHttpResponseHeaders(int, bool) types.Action {
	// 添加响应头
	err := proxywasm.AddHttpResponseHeader("myname", "chengzw")
	if err != nil {
		proxywasm.LogError(err.Error())
	}
	return types.ActionContinue
}
