package bootstrap

import (
	"context"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

func DumpLog(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	klog.Infof("请求方法是: %s\n", info.FullMethod)
	//StripSecrets 是官方的一个工具包 函数
	//返回原始 CSI gRPC 消息的包装器,当消息中包含 譬如秘钥信息时 会
	//把它变成 字符串“***stripped***” 这种格式。防止泄露
	klog.Infof("请求内容是: %s",
		protosanitizer.StripSecrets(req))
	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf("GRPC error: %v", err)
	}
	return resp, err
}
