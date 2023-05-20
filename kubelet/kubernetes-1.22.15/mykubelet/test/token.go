package main

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	bootstraptokenv1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/bootstraptoken/v1"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"log"
)

/**
* @description 创建 Bootstrap Token
* @author chengzw
* @since 2023/5/20
* @link
 */

func main() {
	// id.secret
	token, _ := bootstraputil.GenerateBootstrapToken()
	fmt.Println(token)
	bts, _ := bootstraptokenv1.NewBootstrapTokenString(token)
	fmt.Println(bts)

	// 搞定了默认参数
	opt := options.NewBootstrapTokenOptions()
	opt.Token = bts
	fmt.Println(opt.BootstrapToken)

	// 生成带有默认内容的 secret
	bootSecret := bootstraptokenv1.BootstrapTokenToSecret(opt.BootstrapToken)
	restConfig, err := clientcmd.BuildConfigFromFlags("", homedir.HomeDir()+"/.kube/config")
	if err != nil {
		log.Fatalln(err)
	}

	client, err := clientset.NewForConfig(restConfig)
	createdSecret, err := client.CoreV1().Secrets(metav1.NamespaceSystem).Create(context.TODO(), bootSecret, metav1.CreateOptions{})
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println("secret 创建成功:", createdSecret.Name)
}
