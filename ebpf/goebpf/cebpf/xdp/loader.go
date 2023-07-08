package xdp

//go:generate bpf2go -cc $BPF_CLANG -cflags $BPF_CFLAGS myxdp xdp.bpf.c -- -I ../headers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"log"
	"net"
	"unsafe"
)

type IpData struct {
	SIP   uint32
	DIP   uint32
	PktSz uint32
	III   uint32
	Sport uint16
	Dport uint16
}

func ntohs(port uint16) uint16 {
	return ((port & 0xff) << 8) | (port >> 8)
}

func resolveIP(input_ip uint32, isbig bool) net.IP {
	ipNetworkOrder := make([]byte, 4)
	if isbig {
		binary.BigEndian.PutUint32(ipNetworkOrder, input_ip)
	} else {
		binary.LittleEndian.PutUint32(ipNetworkOrder, input_ip)
	}

	return ipNetworkOrder
}

func LoadXDP() {

	xdpObj := myxdpObjects{}
	err := loadMyxdpObjects(&xdpObj, nil)
	if err != nil {
		log.Fatalln("加载出错:", err)
	}

	defer xdpObj.Close()
	iface, err := net.InterfaceByName("docker0")
	if err != nil {
		log.Fatalln(err)
	}

	// 将 xdp 程序挂载到网卡上
	l, err := link.AttachXDP(link.XDPOptions{
		Program:   xdpObj.MyPass,
		Interface: iface.Index,
	})
	if err != nil {
		log.Fatalln(err)
	}
	defer l.Close()

	//创建reader 用来读取  内核Map
	rd, err := ringbuf.NewReader(xdpObj.IpMap)

	if err != nil {
		log.Fatalf("creating event reader: %s", err)
	}
	defer rd.Close()

	fmt.Println("开始监听xdp")
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
			data := (*IpData)(unsafe.Pointer(&record.RawSample[0]))

			// 转换为网络字节序
			ipAddr1 := resolveIP(data.SIP, true)
			ipAddr2 := resolveIP(data.DIP, true)
			fmt.Printf("来源IP:%s,目标IP:%s,包大小:%d,入口网卡index:%d,来源端口:%d,目标端口:%d\n",
				ipAddr1.To4().String(),
				ipAddr2.To4().String(),
				data.PktSz,
				data.III, data.Sport, data.Dport)

		}
	}
}
