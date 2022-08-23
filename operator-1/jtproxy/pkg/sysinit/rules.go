package sysinit

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/valyala/fasthttp"
	"github.com/yeqown/fasthttp-reverse-proxy/v2"
	v1 "k8s.io/api/networking/v1"
	"net/http"
	"net/url"
)

type ProxyHandler struct {
	Proxy *proxy.ReverseProxy // proxy对象。 保存proxy
}

//空函数没啥用
func (this *ProxyHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

var MyRouter *mux.Router

func init() {
	MyRouter = mux.NewRouter()
}

//解析配置文件中的rules， 初始化 路由
func ParseRule() {
	for _, rule := range SysConfig.Ingress.Rules {

		for _, path := range rule.HTTP.Paths {
			//构建 反代对象
			rProxy := proxy.NewReverseProxy(
				fmt.Sprintf("%s:%d", path.Backend.Service.Name, path.Backend.Service.Port.Number))
			//本课程来自程序员在囧途(www.jtthink.com)咨询群：98514334
			if path.PathType != nil && *path.PathType == v1.PathTypeExact {
				MyRouter.NewRoute().Path(path.Path). //精确匹配
									Methods("GET", "POST", "PUT", "DELETE", "OPTIONS").
									Handler(&ProxyHandler{Proxy: rProxy})
			} else {
				MyRouter.NewRoute().PathPrefix(path.Path).
					Methods("GET", "POST", "PUT", "DELETE", "OPTIONS").
					Handler(&ProxyHandler{Proxy: rProxy})
			}

		}
	}
}

// 获取路由   （先匹配 请求path ，如果匹配到 ，会返回 对应的proxy 对象)
func GetRoute(req fasthttp.Request) *proxy.ReverseProxy {
	match := &mux.RouteMatch{}
	httpReq := &http.Request{URL: &url.URL{Path: string(req.URI().Path())}, Method: string(req.Header.Method())}
	if MyRouter.Match(httpReq, match) {
		return match.Handler.(*ProxyHandler).Proxy
	}
	return nil
}

//本课程来自程序员在囧途(www.jtthink.com)咨询群：98514334
