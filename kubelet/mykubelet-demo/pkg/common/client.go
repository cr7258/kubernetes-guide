package common

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"mykubelet/pkg/util"
	"net/url"
)

func NewForKubeletConfig() (*kubernetes.Clientset, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", util.KUBLET_CONFIG)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(restCfg)

	if err != nil {
		return nil, err
	}
	return client, nil
}

// 根据 token创建 低权限的client ，注意这里为了方便。 直接insecure 跳过ca
func NewForBootStrapToken(token string, masterUrl string) *kubernetes.Clientset {
	urlObj, err := url.Parse(masterUrl)
	if err != nil {
		klog.Fatalln(err)
	}
	restConfig := rest.Config{
		BearerToken: token,
		Host:        urlObj.Host,
		APIPath:     urlObj.Path,
	}
	// 主要是为了简化 连接。 大家可以执行修改，加上 ca
	restConfig.Insecure = true
	client, err := kubernetes.NewForConfig(&restConfig)

	if err != nil {
		klog.Fatalln(err)
	}
	klog.V(3).Info("create clientset by bootstrap token")
	return client
}
