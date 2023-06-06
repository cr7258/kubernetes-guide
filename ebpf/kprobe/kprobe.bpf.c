#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <linux/limits.h> // 定义了NAME_MAX
#include <stddef.h> // size_t 无符号整数类型，通常被用来表示内存块的大小或元素的数量
char LICENSE[] SEC("license") = "Dual BSD/GPL";
typedef unsigned int u32;
struct data_t {
    u32 pid;
    char comm[NAME_MAX];  //NAME_MAX 文件名的最大长度，通常也可以用于进程或线程名称的最大长度
};
SEC("kprobe/__x64_sys_write")
int kprobe_write(struct pt_regs *ctx, int fd, const void *buf, size_t count)
{
    struct data_t data = {};
    data.pid = bpf_get_current_pid_tgid() >> 32;
    if (data.pid!=965676){ // 替换成 Go 程序的 PID
        return 0;
    }
    bpf_get_current_comm(&data.comm, sizeof(data.comm));
    bpf_printk("pid= %d,name:%s. writing data\n",  data.pid, data.comm);
    return 0;
}