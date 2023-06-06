## 创建 Linux 虚拟机

```bash
# 启动虚拟机
limactl start ebpf.yaml

# 进入虚拟机
limactl shell ebpf
```

## 启动第一个 eBPF 程序

eunomia-bpf 是一个开源的 eBPF 动态加载运行时和开发工具链，是为了简化 eBPF 程序的开发、构建、分发、运行而设计的，基于 libbpf 的 CO-RE 轻量级开发框架。

我们会先从一个简单的 eBPF 程序开始 ([minimal.bpf.c](quickstart/minimal.bpf.c))，它会在内核中打印一条消息。我们会使用 eunomia-bpf 的编译器工具链将其编译为 bpf 字节码文件，然后使用 ecli 工具加载并运行该程序。作为示例，我们可以暂时省略用户态程序的部分。

这段程序通过定义一个 handle_tp 函数并使用 SEC 宏把它附加到 sys_enter_write tracepoint（即在进入 write 系统调用时执行）。该函数通过使用 bpf_get_current_pid_tgid 和 bpf_printk 函数获取调用 write 系统调用的进程 ID，并在内核日志中打印出来。
- bpf_trace_printk()： 一种将信息输出到 trace_pipe(/sys/kernel/debug/tracing/trace_pipe)简单机制。 在一些简单用例中这样使用没有问题， 但它也有一些限制：最多3 参数； 第一个参数必须是%s(即字符串)；同时trace_pipe在内核中全局共享，其他并行使用trace_pipe的程序有可能会将 trace_pipe 的输出扰乱。 一个更好的方式是通过 BPF_PERF_OUTPUT(), 稍后将会讲到。
- void *ctx：ctx 本来是具体类型的参数， 但是由于我们这里没有使用这个参数，因此就将其写成void *类型。
- return 0;：必须这样，返回 0 (如果要知道原因, 参考 #139 https://github.com/iovisor/bcc/issues/139)。

要编译和运行这段程序，可以使用 ecc 工具和 ecli 命令。首先使用 ecc 编译程序：

```
cd quickstart
docker run -it -v `pwd`/:/src/ yunwei37/ebpm:latest
```

然后使用 ecli 运行编译后的程序，编译后会生成 package.json 和 minimal.skel.json 两个文件。

```
$ sudo ecli run ./package.json
Runing eBPF program...
```

运行这段程序后，可以通过查看 /sys/kernel/debug/tracing/trace_pipe 文件来查看 eBPF 程序的输出：

```
$ sudo cat /sys/kernel/debug/tracing/trace_pipe
<...>-3840345 [010] d... 3220701.101143: bpf_trace_printk: write system call from PID 3840345.
<...>-3840345 [010] d... 3220701.101143: bpf_trace_printk: write system call from
```

## tracepoint 监控 Go 程序写入

启动 Go 程序执行写入操作。

```bash
go run testwrite/main.go
当前的PID是： 965676
写入成功 2023-06-06 14:24:05.536120899 +0000 UTC m=+0.000339041
写入成功 2023-06-06 14:24:10.538152765 +0000 UTC m=+5.002370896
```

修改 tracepoint/test.bpf.c 文件，只对指定 PID 的程序进行跟踪。

```bash
const int myappid=965676; // 替换成 Go 程序的 PID
```

使用 ecc 编译程序：

```bash
cd tracepoint
docker run -it -v `pwd`/:/src/ yunwei37/ebpm:latest
```

编译出来后执行：

```bash
ecli run ./package.json
```

查看效果：

```bash
cat /sys/kernel/debug/tracing/trace_pipe

# 输出
main-965676  [001] d...1 1730125.361244: bpf_trace_printk: jtthink-BPF triggered from PID 965676.

main-965676  [001] d...1 1730125.361303: bpf_trace_printk: jtthink-BPF triggered from PID 965676.

main-965676  [001] d...1 1730130.361559: bpf_trace_printk: jtthink-BPF triggered from PID 965676.

main-965676  [001] d...1 1730130.361612: bpf_trace_printk: jtthink-BPF triggered from PID 965676.
```

## kprobe 监控 Go 程序写入

kprobes ：动态内核跟踪技术，可以定义自己的回调函数，在内核几乎所有的函数中动态地插入探测点，当内核执行流程执行到指定的探测函数时，会调用该回调函数，用户即可收集所需的信息。虽然灵活，但是可能相对 tracepoint (预定义跟踪点进行采样)有性能影响。
我们可以通过 `cat /proc/kallsyms` 来查看内核函数，至于函数体可能需要查看你所因此内核的源码。


继续使用 kprobe 来监控上面运行的 Go 程序，修改 tracepoint/test.bpf.c 文件，只对指定 PID 的程序进行跟踪。

```bash
if (data.pid!=965676){ // 替换成 Go 程序的 PID 
        return 0;
    }
```

使用 ecc 编译程序：

```bash
cd kprobe
docker run -it -v `pwd`/:/src/ yunwei37/ebpm:latest
```

编译出来后执行：

```bash
ecli run ./package.json
```

查看效果：

```bash
cat /sys/kernel/debug/tracing/trace_pipe

# 输出
main-965676  [000] d...1 1730410.560730: bpf_trace_printk: pid= 965676,name:main. writing data

main-965676  [000] d...1 1730410.560773: bpf_trace_printk: pid= 965676,name:main. writing data

main-965676  [000] d...1 1730415.564056: bpf_trace_printk: pid= 965676,name:main. writing data

main-965676  [000] d...1 1730415.564105: bpf_trace_printk: pid= 965676,name:main. writing data
```
## XDP 拦截 ICMP 协议

xdp_md 在头文件 /usr/include/linux/bpf.h 有定义：
- data： 数据包数据的地址。 指向数据包数据的开头。
- data_end： 数据包数据的结束地址。指向数据包的结尾。
- data_meta： 数据包元数据的地址。存储有关数据包的附加信息。
- ingress_ifindex： 接收数据包的网络接口的索引。
- rx_queue_index： 接收数据包的接收队列的索引。

头文件中定义 if_ether.h。它代表以太网链路层报头。其主要目的是定义以太网报头的结构，其中包括源 MAC 地址和目的 MAC 地址，以及以太网协议类型。

```c
struct ethhdr
{
unsigned char h_dest[ETH_ALEN]; //目的MAC地址
unsigned char h_source[ETH_ALEN]; //源MAC地址
__u16 h_proto ; //网络层所使用的协议类型
}__attribute__((packed))
```

头文件 iphdr 中定义 <linux/ip.h>。它用于描述 IPv4 数据包的 IP 标头。该结构包括 IP 版本、报头长度、服务类型、数据包总长度、标识号、标志、生存时间、协议、校验和、源 IP 地址和目标 IP 地址等字段。协议常见数值：
- 1：ICMP（Internet 控制报文协议）
- 2：IGMP（Internet 组管理协议）
- 6：TCP（传输控制协议）
- 17：UDP（用户数据报协议）
- 41：IPv6 封装的 IPv6 数据报
- 47：GRE（通用路由封装）
- 50：ESP（封装安全载荷）
- 51：AH（认证头部）
- 89：OSPF（开放式最短路径优先协议）
- 132：SCTP（流控制传输协议）

使用 ecc 编译程序：

```bash
cd xdp
docker run -it -v `pwd`/:/src/ yunwei37/ebpm:latest
```

启用 XDP，使用 docker0 或者自己创建网桥来处理，不要对 eth0 下手，以免连不上主机。

```bash
# ip link set dev <网卡> xdp obj <文件名> sec xdp verbose
ip link set dev docker0 xdp obj xdp.bpf.o sec xdp verbose
```

卸载 XDP：

```bash
ip link set dev docker0 xdp off
```
