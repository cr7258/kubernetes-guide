package ringbuffer

//go:generate bpf2go -cc $BPF_CLANG -cflags $BPF_CFLAGS tc_write tc_write.bpf.c -- -I ../headers
