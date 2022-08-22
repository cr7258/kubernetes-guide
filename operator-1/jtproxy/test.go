package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"net/url"
)

func main() {


	r:=mux.NewRouter()
	r.NewRoute().Path("/").Methods("GET")

	r.NewRoute().Path("/users/{id:\\d+}").Methods("GET","POST","PUT","DELETE","OPTIONS")

	match:=&mux.RouteMatch{}

	req:=&http.Request{URL:&url.URL{Path: "/users/abc"},Method:"GET"}
	fmt.Println(r.Match(req,match))

}
