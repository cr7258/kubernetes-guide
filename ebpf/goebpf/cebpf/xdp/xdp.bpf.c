//go:build ignore
#include <common.h>
#include <linux/tcp.h>
#include <bpf_endian.h>


struct ip_data {
     __u32 sip; //来源IP
     __u32 dip; //目标IP
     __u32 pkt_sz;//包大小
     __u32 iii;//  ingress_ifindex
     __be16  sport; //来源端口
     __be16  dport; //目的端口
  };
 struct {
     __uint(type, BPF_MAP_TYPE_RINGBUF);
     __uint(max_entries,1<<20);
 } ip_map SEC(".maps");

 // IP 白名单
struct bpf_map_def SEC("maps") allow_ips_map = {
     .type = BPF_MAP_TYPE_HASH,
     .key_size = sizeof(__u32),
     .value_size = sizeof(__u8), // 设置为 1 表示允许放行
     .max_entries = 1024,
 };

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

    struct iphdr *ip = data + sizeof(*eth); // 得到了 ip 层
    if ((void*)ip + sizeof(*ip) > data_end) {
        bpf_printk("Invalid IP header\n");
        return XDP_DROP;
    }
     if (ip->protocol != 6) { //如果不是TCP 就不处理了
        return XDP_PASS;
     }
     struct tcphdr *tcp = (void*)ip + sizeof(*ip);
      if ((void*)tcp + sizeof(*tcp) > data_end) {
            return XDP_DROP;
      }
    //下面构建数据发送的内容
     struct ip_data *ipdata = NULL;
     ipdata=bpf_ringbuf_reserve(&ip_map, sizeof(*ipdata), 0);
     if(!ipdata){
       return XDP_PASS;
     }
    ipdata->sip=bpf_ntohl(ip->saddr);// 网络字节序 转换成 主机字节序  32位
    ipdata->dip=bpf_ntohl(ip->daddr);
    ipdata->pkt_sz=pkt_sz;
    ipdata->iii=ctx->ingress_ifindex;
    ipdata->sport=bpf_ntohs(tcp->source); //16位
    ipdata->dport=bpf_ntohs(tcp->dest);

    bpf_ringbuf_submit(ipdata, 0);

    __u32 sip=bpf_ntohl(ip->saddr);
    __u8 *allow=bpf_map_lookup_elem(&allow_ips_map, &sip);
    if(allow && *allow==1){ // 在 loader.go 的 initAllowIpMap 方法中会将允许的 IP 地址设置为 1
      return XDP_PASS;
    }

    return XDP_DROP;
}

char __license[] SEC("license") = "GPL";