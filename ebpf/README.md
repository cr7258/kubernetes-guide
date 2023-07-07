* [创建 Linux 虚拟机](#创建-linux-虚拟机)
* [启动第一个 eBPF 程序](#启动第一个-ebpf-程序)
* [tracepoint 监控 Go 程序写入](#tracepoint-监控-go-程序写入)
* [kprobe 监控 Go 程序写入](#kprobe-监控-go-程序写入)
* [XDP 拦截 ICMP 协议](#xdp-拦截-icmp-协议)

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

启用 XDP 之前先创建一个容器。

```bash
docker run -itd --name nginx nginx
```

获取容器的 IP 地址。

```bash
docker inspect -f "{{ .NetworkSettings.IPAddress }}" nginx

# 输出
172.17.0.3
```

此时 curl 和 ping 容器都是可以成功访问的。

```bash
> curl 172.17.0.3

<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
html { color-scheme: light dark; }
body { width: 35em; margin: 0 auto;
font-family: Tahoma, Verdana, Arial, sans-serif; }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>

> ping 172.17.0.3
PING 172.17.0.3 (172.17.0.3) 56(84) bytes of data.
64 bytes from 172.17.0.3: icmp_seq=1 ttl=64 time=0.059 ms
64 bytes from 172.17.0.3: icmp_seq=2 ttl=64 time=0.058 ms
64 bytes from 172.17.0.3: icmp_seq=3 ttl=64 time=0.044 ms
```

启用 XDP，使用 docker0 或者自己创建网桥来处理，不要对 eth0 下手，以免连不上主机。

```bash
# ip link set dev <网卡> xdp obj <文件名> sec xdp verbose
ip link set dev docker0 xdp obj xdp.bpf.o sec xdp verbose
```

执行 ip link 命令可以看到 docker0 网卡上挂载了 XDP 程序。

```bash
3: docker0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 xdpgeneric qdisc noqueue state UP mode DEFAULT group default
    link/ether 02:42:75:d8:d6:43 brd ff:ff:ff:ff:ff:ff
    prog/xdp id 29200 tag d0ecfbec9b51b126 jited
```

此时再次访问容器，会发现无法通过 Ping 访问容器了，但是 curl 还是可以访问容器。

```bash
> ping 172.17.0.3
PING 172.17.0.3 (172.17.0.3) 56(84) bytes of data.
^C
--- 172.17.0.3 ping statistics ---
45 packets transmitted, 0 received, 100% packet loss, time 45051ms

> curl 172.17.0.3
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
html { color-scheme: light dark; }
body { width: 35em; margin: 0 auto;
font-family: Tahoma, Verdana, Arial, sans-serif; }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
```

查看 trace 日志。

```bash
cat /sys/kernel/debug/tracing/trace_pipe

# 输出
ping-152078  [000] d.s11 1732113.788260: bpf_trace_printk: Drop ICMP packets

ping-969242  [001] d.s11 1732114.208199: bpf_trace_printk: Drop ICMP packets

ping-152078  [000] d.s11 1732114.788594: bpf_trace_printk: Drop ICMP packets

ping-969242  [000] d.s11 1732115.232141: bpf_trace_printk: Drop ICMP packets
```

卸载 XDP：

```bash
ip link set dev docker0 xdp off
```

## 用户态开发 cilium/ebpf 入门

安装 bpf2go 工具，执行 go install 后 bpf2go 会安装到 $GOPATH/bin 目录下。

```bash
go get github.com/cilium/ebpf/cmd/bpf2go
go install github.com/cilium/ebpf/cmd/bpf2go
```

安装依赖，参考资料：https://www.ghl.name/archives/how-to-fix-asm-types-h-no-found.html

```bash
apt install -y libelf-dev llvm

# 安装 clang-14
apt install -y lsb-release wget software-properties-common gnupg
wget https://apt.llvm.org/llvm.sh
chmod +x llvm.sh
sudo ./llvm.sh 14 all
# 上一步安装过程中会安装无用的 clang-11 的包，这一条命令是用来卸载它们的
sudo apt autoremove

# 设置 clang 的默认版本为 clang-14
update-alternatives --install /usr/bin/llc llc /usr/bin/llc-14 100
update-alternatives --install /usr/bin/clang clang /usr/bin/clang-14 100

apt install -y gcc-multilib
```

查看 clang 版本：

```bash
# clang -v
Ubuntu clang version 14.0.6
Target: aarch64-unknown-linux-gnu
Thread model: posix
InstalledDir: /usr/bin
Found candidate GCC installation: /usr/bin/../lib/gcc/aarch64-linux-gnu/9
Selected GCC installation: /usr/bin/../lib/gcc/aarch64-linux-gnu/9
Candidate multilib: .;@m64
Selected multilib: .;@m64
```

在 goebpf/cebpf/tc 目录中的 tc_write.bpf.c 文件是 eBPF 程序，doc.go 中 bpf2go 命令根据 tc_write.bpf.c 文件生成相关的 o 文件和 go 文件。

```bash
//go:generate bpf2go -cc $BPF_CLANG -cflags $BPF_CFLAGS tc_write tc_write.bpf.c -- -I $BPF_HEADERS
```

在项目根目录执行 make 命令生成文件。

```bash
make

# 输出
go generate ./...
Compiled /root/kubernetes-guide/ebpf/goebpf/cebpf/tc/tc_write_bpfel.o
Stripped /root/kubernetes-guide/ebpf/goebpf/cebpf/tc/tc_write_bpfel.o
Wrote /root/kubernetes-guide/ebpf/goebpf/cebpf/tc/tc_write_bpfel.go
Compiled /root/kubernetes-guide/ebpf/goebpf/cebpf/tc/tc_write_bpfeb.o
Stripped /root/kubernetes-guide/ebpf/goebpf/cebpf/tc/tc_write_bpfeb.o
Wrote /root/kubernetes-guide/ebpf/goebpf/cebpf/tc/tc_write_bpfeb.go
```

启动 eBPF 程序。

```bash
cd goebpf/cmd/tc/
go run main.go
```

启动被监控的程序。

```bash
cd testwrite
./testwrite
```

查看监听的 trace 日志。

```bash
# cat /sys/kernel/debug/tracing/trace_pipe
testwrite-773766  [000] d...1 1084879.915646: bpf_trace_printk: pid= 773766,name:testwrite. writing data

testwrite-773766  [000] d...1 1084879.919001: bpf_trace_printk: pid= 773766,name:testwrite. writing data

testwrite-773766  [000] d...1 1084879.919241: bpf_trace_printk: pid= 773766,name:testwrite. writing data
```

## eBPF Maps 入门

### 基本创建，接收事件

cilium/ebpf 提供了一些头文件 https://github.com/cilium/ebpf/tree/master/examples/headers ，下载以后放到 goebpf/cebpf/headers 目录中。

之前我们在 [tc_write.bpf.c](goebpf/cebpf/tc/tc_write.bpf.c) 文件中引用的头文件可以直接改成 `#include <common.h>`。

```c
//#include <bpf/bpf_helpers.h>
//#include <bpf/bpf_tracing.h>
//#include <linux/limits.h>
#include <common.h>
```

Map 是用户空间和内核空间进行数据交换、信息传递的桥梁，它以 key/value 方式将数据存储在内核中，可以被任何知道它们的 BPF 程序访问。 

需要包含 <linux/bpf.h>，使用 SEC 语法糖创建：

```c
struct bpf_map_def SEC("maps") my_bpf_map = {
.type       = BPF_MAP_TYPE_HASH,
.key_size   = sizeof(int),
.value_size   = sizeof(int),
.max_entries = 100,
.map_flags   = BPF_F_NO_PREALLOC,
};
```

场景的 Map 类型：
- BPF_MAP_TYPE_HASH：哈希表类型的 map，可以用于快速存取键值对，适用于高速数据查询场景。
- BPF_MAP_TYPE_ARRAY：数组类型的 map，可以用于存储连续的数据元素，适用于按照顺序访问数据的场景。 
- BPF_MAP_TYPE_PROG_ARRAY：程序数组类型的 map，可以用于存储 eBPF 程序，适用于动态加载和卸载 eBPF 程序的场景。 
- BPF_MAP_TYPE_PERF_EVENT_ARRAY：性能事件数组类型的 map，可以用于收集系统性能数据，例如 CPU 使用率、内存使用率等。 
- BPF_MAP_TYPE_STACK_TRACE：堆栈跟踪类型的 map，可以用于跟踪函数调用堆栈，适用于调试和性能分析场景。 
- BPF_MAP_TYPE_LRU_HASH：LRU哈希表类型的 map，可以用于快速存取键值对，并在内存不足时自动删除最近最少使用的键值对，适用于高速数据查询场景。 
- BPF_MAP_TYPE_LRU_PERCPU_HASH：LRU 哈希表类型的 map，支持多个 CPU 核心并发访问，适用于高速数据查询和多线程场景。 
- BPF_MAP_TYPE_ARRAY_OF_MAPS：map数组类型的 map，可以用于存储其他类型的 map，适用于复杂数据结构的场景。 
- BPF_MAP_TYPE_DEVMAP：设备映射类型的 map，可以将网络设备和 eBPF 程序绑定，适用于网络协议栈优化和网络安全场景。 
- BPF_MAP_TYPE_CPUMAP：CPU 映射类型的 map，可以将 eBPF 程序和 CPU 核心绑定，适用于多核心系统性能优化场景。

在 [tc_write.bpf.c](goebpf/cebpf/tc/tc_write.bpf.c) 文件中创建一个 Map，类型是 BPF_MAP_TYPE_PERF_EVENT_ARRAY。

```c
struct bpf_map_def SEC("maps") log_map = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(int),
    .value_size = sizeof(__u32),
    .max_entries = 0,
};
```

向用户态发送程序，BPF_MAP_TYPE_PERF_EVENT_ARRAY 的核心函数是 bpf_perf_event_output()。

```c
bpf_perf_event_output(ctx, &log_map, 0, &data, sizeof(data));
```