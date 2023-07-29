package lib

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"path/filepath"
)

/**
* @description
* @author chengzw
* @since 2023/7/29
* @link
 */

func Check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func K8sRestConfig() *rest.Config {
	homedir := homedir.HomeDir()
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(homedir, ".kube", "config"))
	if err != nil {
		log.Fatal(err)
	}
	return config
}
