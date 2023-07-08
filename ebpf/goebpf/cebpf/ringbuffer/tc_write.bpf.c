//go:build ignore
#include "common.h"
#include <linux/limits.h>
typedef unsigned int u32;

char LICENSE[] SEC("license") = "GPL";
int is_eq(char *str1,char *str2){
    int eq=1;
     int i ;
     for (i=0;i<sizeof(str1)-1 && i<sizeof(str2)-1;i++){
         if (str1[i]!=str2[i]){
            eq=0;
            break;;
         }
     }
     return eq;
}
// 定义一个结构体，用于pid和进程名称
struct data_t {
    __u32 pid;
    char comm[256];  //NAME_MAX 文件名的最大长度，通常也可以用于进程或线程名称的最大长度
};

// 创建map
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries,1<<20);
} log_map SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_write")
int handle_tp(void *ctx)
{
    // 方式一：bpf_ringbuf_output
    // struct data_t data = {};
    // data.pid = bpf_get_current_pid_tgid() >> 32; //获取PID
    // bpf_get_current_comm(&data.comm, sizeof(data.comm)); //获取进程名称
    // bpf_ringbuf_output(&log_map, &data, sizeof(data), 0);

    // 方式一：bpf_ringbuf_reserve()/bpf_ringbuf_commit()
    struct data_t *data = NULL;
    data=bpf_ringbuf_reserve(&log_map, sizeof(*data), 0);
    if(!data){
      return 0;
    }
    data->pid = bpf_get_current_pid_tgid() >> 32; //获取PID
    bpf_get_current_comm(&data->comm, sizeof(data->comm)); //获取进程名称
    bpf_ringbuf_submit(data, 0); // 向用户态发送数据

   return 0;
}