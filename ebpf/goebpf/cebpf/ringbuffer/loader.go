package ringbuffer

import (
	"bytes"
	"errors"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"log"
	"unsafe"
)

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
		log.Fatal("加载出错:", err)
	}
	defer tc_obj.Close()
	// TODO 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
	tp, err := link.Tracepoint("syscalls", "sys_enter_openat",
		tc_obj.HandleTp, nil)
	if err != nil {
		log.Fatalf("opening tracepoint: %s", err)
	}
	defer tp.Close()

	//创建reader 用来读取  内核Map
	rd, err := ringbuf.NewReader(tc_obj.LogMap)

	if err != nil {
		log.Fatalf("creating event reader: %s", err)
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
}
