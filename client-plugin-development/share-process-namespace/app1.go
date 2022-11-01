package main

import (
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
)

var counter = 1
func main() {
    http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
        writer.Write([]byte("this is app1\n"))
    })
	http.HandleFunc("/counter", func(writer http.ResponseWriter, request *http.Request) {
		counter++
		writer.Write([]byte(fmt.Sprintf("counter is %d", counter)))
    })

	go func() {
		for {
			ch := make(chan os.Signal)
			// SIGUSR1 is User-defined signal
			signal.Notify(ch, syscall.SIGTERM, syscall.SIGUSR1)
			c := <- ch
			switch c {
			case syscall.SIGUSR1:
                fmt.Println("counter is reset")
				counter = 0
			}
			time.Sleep(time.Millisecond * 20)
		}
	}()
	http.ListenAndServe(":8080", nil)
}