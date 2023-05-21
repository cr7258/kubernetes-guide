package main

import (
	"flag"
	"k8s.io/klog/v2"
	"mykubelet/pkg/bootstrap"
	"mykubelet/pkg/common"
	"mykubelet/pkg/node"
)

const (
	NODENAME  = "myk8s"                   // 节点名字
	MASTERURL = "https://127.0.0.1:38183" // API Server 地址
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	// kubeadm token create 命令创建
	token := "x2jft0.5isyxfdbiyfqasdb"
	// 通过 Token 获取低权限的用户，可以创建 CertificateSigningRequest 对象
	// 等待 CSR 批复，提取证书，并在本地生成 Kubelet 使用的 kubeconfig 文件
	bootstrap.BootStrap(token, NODENAME, MASTERURL)

	// 使用 kubeconfig 文件生成 clientset
	kubeCient, err := common.NewForKubeletConfig()
	if err != nil {
		klog.Fatalln(err)
	}

	// 创建 Node 对象
	node.RegisterNode(NODENAME, kubeCient)

	//开启租约控制器
	node.StartLeaseController(kubeCient, NODENAME)
}
