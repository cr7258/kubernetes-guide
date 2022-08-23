package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"net/url"
	"regexp"
)

func main() {



	r:=mux.NewRouter()
	r.NewRoute().Path("/").Methods("GET")

	r.NewRoute().PathPrefix("/jtthink/{param:.*}").Methods("GET","POST","PUT","DELETE","OPTIONS")

	match:=&mux.RouteMatch{}

	reqPath:="/jtthink/abc"
	req:=&http.Request{URL:&url.URL{Path:reqPath },Method:"GET"}

	if r.Match(req,match){
		fmt.Println(match.Route.GetPathRegexp())
		pathExp,_:=match.Route.GetPathRegexp()
		reg:=regexp.MustCompile(pathExp)
		fmt.Println(reg.ReplaceAllString(reqPath,"/$1"))
	}



}
