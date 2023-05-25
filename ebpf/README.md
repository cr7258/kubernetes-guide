## 创建 Linux 虚拟机

```bash
# 启动虚拟机
limactl start ebpf.yaml

# 进入虚拟机
limactl shell ebpf
```

## 启动第一个 eBPF 程序

eunomia-bpf 是一个开源的 eBPF 动态加载运行时和开发工具链，是为了简化 eBPF 程序的开发、构建、分发、运行而设计的，基于 libbpf 的 CO-RE 轻量级开发框架。

我们会先从一个简单的 eBPF 程序开始 ([minimal.bpf.c](./minimal.bpf.c))，它会在内核中打印一条消息。我们会使用 eunomia-bpf 的编译器工具链将其编译为 bpf 字节码文件，然后使用 ecli 工具加载并运行该程序。作为示例，我们可以暂时省略用户态程序的部分。

这段程序通过定义一个 handle_tp 函数并使用 SEC 宏把它附加到 sys_enter_write tracepoint（即在进入 write 系统调用时执行）。该函数通过使用 bpf_get_current_pid_tgid 和 bpf_printk 函数获取调用 write 系统调用的进程 ID，并在内核日志中打印出来。
- bpf_trace_printk()： 一种将信息输出到 trace_pipe(/sys/kernel/debug/tracing/trace_pipe)简单机制。 在一些简单用例中这样使用没有问题， 但它也有一些限制：最多3 参数； 第一个参数必须是%s(即字符串)；同时trace_pipe在内核中全局共享，其他并行使用trace_pipe的程序有可能会将 trace_pipe 的输出扰乱。 一个更好的方式是通过 BPF_PERF_OUTPUT(), 稍后将会讲到。
- void *ctx：ctx 本来是具体类型的参数， 但是由于我们这里没有使用这个参数，因此就将其写成void *类型。
- return 0;：必须这样，返回 0 (如果要知道原因, 参考 #139 https://github.com/iovisor/bcc/issues/139)。

要编译和运行这段程序，可以使用 ecc 工具和 ecli 命令。首先使用 ecc 编译程序：

```
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
