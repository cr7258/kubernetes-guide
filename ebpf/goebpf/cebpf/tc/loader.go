package tc

import (
	"bytes"
	"errors"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"log"
	"os"
	"time"
	"unsafe"
)

// 在用户态需要定义 struct 匹配内核态中的 Map
type DataT struct {
	Pid  uint32
	Comm [256]byte
}

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
		if len(record.RawSample) > 0 {
			data := (*DataT)(unsafe.Pointer(&record.RawSample[0]))
			log.Println("进程名:", string(bytes.TrimRight(data.Comm[:], "0x00")))
		}
	}

	time.Sleep(time.Second * 3600)
}
