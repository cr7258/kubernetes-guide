package main

import (
	"fmt"
	"mygoebpf/cebpf/ringbuffer"
)

func main() {
	fmt.Println("开始启动eBPF")
	ringbuffer.LoadTcWrite()
}
