package bootstrap

import (
	"k8s.io/klog/v2"
	"mykubelet/pkg/bootstrap/lib"
	"mykubelet/pkg/common"
	"mykubelet/pkg/util"
)

// 分几步   证书获取 。client 创建 node节点创建
func BootStrap(token, nodeName, masterUrl string) {
	if !util.NeedRequestCSR() {
		klog.Infoln("kubelet.config already exists. skip csr-boot")
		return
	}
	klog.Infoln("begin bootstrap ")
	bootClient := common.NewForBootStrapToken(token, masterUrl)
	csrObj, err := lib.CreateCSRCert(bootClient, nodeName)
	if err != nil {
		klog.Fatalln(err)
	}
	//等待批复 ，超时时间

	err = lib.WaitForCSRApprove(csrObj, lib.CSR_WAITING_TIMEOUT, bootClient)
	if err != nil {
		klog.Fatalln(err)
	}
	klog.Infoln("kubelet pem-files have been saved in .kube ")

	err = lib.GenKubeletConfig(masterUrl)
	if err != nil {
		klog.Fatalln(err)
	}
	klog.Infoln("testing kubeclient")
	client, err := common.NewForKubeletConfig()
	if err != nil {
		klog.Fatalln(err)
	}
	info, err := client.ServerVersion()
	if err != nil {
		klog.Fatalln(err)
	}
	klog.Info(info.String())

}
