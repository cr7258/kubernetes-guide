package main

import (
	"context"
	"fmt"
	clientv1 "github.com/shenyisyn/dbcore/pkg/client/clientset/versioned/typed/dbconfig/v1"
	"github.com/shenyisyn/dbcore/pkg/k8sconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
)

func main() {

	k8sConfig := k8sconfig.K8sRestConfig()

	client, err := clientv1.NewForConfig(k8sConfig)
	if err != nil {
		log.Fatal(err)
	}
	dcList, _ := client.DbConfigs("default").List(context.Background(), metav1.ListOptions{})
	fmt.Println(dcList)
}
