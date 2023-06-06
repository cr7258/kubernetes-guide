#include <linux/bpf.h> 
#include <bpf/bpf_helpers.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>

SEC("xdp")
int my_pass(struct xdp_md* ctx) {
    void *data = (void*)(long)ctx->data;
    void *data_end = (void*)(long)ctx->data_end;
    int pkt_sz = data_end - data;

    struct ethhdr *eth = data;  // 链路层
    if ((void*)eth + sizeof(*eth) > data_end) {  //如果包不完整、或者被篡改， 我们直接DROP
        bpf_printk("Invalid ethernet header\n");
        return XDP_DROP;
    }

    struct iphdr *ip = data + sizeof(*eth); // 得到了 ip层
    if ((void*)ip + sizeof(*ip) > data_end) {
        bpf_printk("Invalid IP header\n");
        return XDP_DROP;
    }

   if (ip->protocol == IPPROTO_ICMP) {
        // 拦截 ICMP Ping 请求
        return XDP_DROP;
    }

    bpf_printk("jtthink output:Packet size is %d, protocol is %d\n", pkt_sz, ip->protocol);
    
    return XDP_PASS;
}

char __license[] SEC("license") = "GPL";