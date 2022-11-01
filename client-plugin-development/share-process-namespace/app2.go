package main

import (
	"net/http"
	"syscall"
)

func main() {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("this is app2\n"))
	})
	http.HandleFunc("/reset", func(writer http.ResponseWriter, request *http.Request) {
		// -1 means send signal to process group, so app1 can receive the signal as well
		syscall.Kill(-1, syscall.SIGUSR1)
	})
	http.ListenAndServe(":8081", nil)
}