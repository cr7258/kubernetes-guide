package sysinit

import (
	"github.com/gorilla/mux"
	"net/http"
)

//route构建器， build 方法必须要执行
var MyRouter *mux.Router

func init() {
	MyRouter = mux.NewRouter()
}

type RouteBuilder struct {
	route *mux.Route
}

func NewRouteBuilder() *RouteBuilder {
	return &RouteBuilder{route: MyRouter.NewRoute()}
}

func (this *RouteBuilder) SetPath(path string, exact bool) *RouteBuilder {
	if exact {
		this.route.Path(path)
	} else {
		this.route.PathPrefix(path)
	}
	return this
}

//第二个参数是故意的，方便调用时传入条件，省的外面写 if else
func (this *RouteBuilder) SetHost(host string, set bool) *RouteBuilder {
	if set {
		this.route.Host(host)
	}
	return this
}
func (this *RouteBuilder) Build(handler http.Handler) {
	this.route.
		Methods("GET", "POST", "PUT", "DELETE", "OPTIONS").
		Handler(handler)
}
