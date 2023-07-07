package tc

import (
	"errors"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"log"
	"os"
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

	// 创建 reader 来读取内核 Map 中的数据
	rd, err := perf.NewReader(tc_obj.LogMap, os.Getpagesize())
	if err != nil {
		log.Fatalf("creating perf event reader: %s", err)
	}
	defer rd.Close()

	for {
		record, err := rd.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				log.Println("Received signal, exiting..")
				return
			}
			log.Printf("reading from reader: %s", err)
			continue
		}
		log.Println("Record:", string(record.RawSample))
	}

	time.Sleep(time.Second * 3600)
}
