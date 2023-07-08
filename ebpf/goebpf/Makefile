CLANG ?= clang
CFLAGS ?= -O2 -g -Wall -Werror

EBPF_ROOT = goebpf/cebpf

all: generate

generate: export BPF_CLANG=$(CLANG)
generate: export BPF_CFLAGS=$(CFLAGS)
generate:
	go generate ./...