//go:build ignore
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
//#include <common.h>
#include <linux/limits.h>
typedef unsigned int u32;


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
    u32 pid;
    char comm[NAME_MAX];  //NAME_MAX 文件名的最大长度，通常也可以用于进程或线程名称的最大长度
};

// 创建map
struct bpf_map_def SEC("maps") log_map = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(int),
    .value_size = sizeof(__u32),
    .max_entries = 0, // 用户态不需要向内核态传递数据，所以这里可以设置为0
};

char LICENSE[] SEC("license") = "Dual BSD/GPL";

SEC("tracepoint/syscalls/sys_enter_write")
int handle_tp(void *ctx)
{ 
    char app_name[]="testwrite";  //这是一个全局变量，编译是会有警告
    struct data_t data = {};
    data.pid = bpf_get_current_pid_tgid() >> 32; //获取PID
    bpf_get_current_comm(&data.comm, sizeof(data.comm)); //获取进程名称
     int eq=is_eq(data.comm,app_name);
    if(eq==1){
        // bpf_printk("pid= %d,name:%s. writing data\n",  data.pid, data.comm);
        // 向用户态发送程序
        bpf_perf_event_output(ctx, &log_map, 0, &data, sizeof(data));
    }
   return 0;
}
