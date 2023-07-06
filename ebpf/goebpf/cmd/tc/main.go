package main

import (
	"fmt"
	"mygoebpf/cebpf/tc"
)

func main() {
	fmt.Println("开始启动eBPF")
	tc.LoadTcWrite()
}
