package main

import (
	"crypto/tls"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/tools/remotecommand"
	"log"
	"net/http"
	"os"
)

func main() {
	execUrl := "http://localhost:6443" // 连接假的 API Server
	req, _ := http.NewRequest("GET", execUrl, nil)
	req.Header.Set("Upgrade", "SPDY/3.1")
	req.Header.Set("Connection", "Upgrade")
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	rt := spdy.NewRoundTripper(tlsConfig, true, false)

	executor, err := remotecommand.NewSPDYExecutorForTransports(rt, rt, http.MethodGet, req.URL)
	if err != nil {
		log.Fatal(err)
	}

	err = executor.Stream(remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    true,
	})
	if err != nil {
		log.Fatalln(err)
	}

}
