package main

import (
	"fmt"
	"mygoebpf/cebpf/xdp"
)

func main() {
	fmt.Println("开始启动eBPF")
	xdp.LoadXDP()
}
