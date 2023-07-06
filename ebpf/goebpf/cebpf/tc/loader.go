package tc

import (
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"log"
	"time"
)

func LoadTcWrite() {
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal(err)
	}

	tc_obj := tc_writeObjects{}
	err := loadTc_writeObjects(&tc_obj, nil)
	if err != nil {
		log.Fatal(err)
	}
	tp, err := link.Tracepoint("syscalls", "sys_enter_openat",
		tc_obj.HandleTp, nil)
	if err != nil {
		log.Fatalf("opening tracepoint: %s", err)
	}
	defer tp.Close()

	time.Sleep(time.Second * 3600)
}
