package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Println(request.Header)
		writer.Write([]byte("ok"))
	})
	http.ListenAndServe(":8080", nil)
}
