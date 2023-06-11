package main

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"log"
	"time"
)

/**
* @description 测试链接 Containerd
* @author chengzw
* @since 2023/6/11
* @link
 */

func initClient() *grpc.ClientConn {
	gopts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "192.168.2.150:8989", gopts...) // 替换成 Containerd 服务器端的 IP
	if err != nil {
		log.Fatalln(err)
	}
	return conn
}

func main() {
	c := initClient()
	cc := v1alpha2.NewRuntimeServiceClient(c)
	rsp, err := cc.Version(context.Background(), &v1alpha2.VersionRequest{})
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(rsp.Version)
}
