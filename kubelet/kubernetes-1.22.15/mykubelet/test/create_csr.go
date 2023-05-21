package main

import (
	"io/ioutil"
	v1 "k8s.io/api/certificates/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubernetes/mykubelet/mylib"
	"log"
)

/**
* @description 创建 CertificateSigningRequest 对象，保存 Private Key 到本地
* @author chengzw
* @since 2023/5/21
* @link
 */

func checkerr(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func main() {
	restConfig, err := clientcmd.BuildConfigFromFlags("", homedir.HomeDir()+"/.kube/config")
	if err != nil {
		log.Fatalln(err)
	}
	client, err := clientset.NewForConfig(restConfig)
	if err != nil {
		log.Fatalln(err)
	}

	ch := make(chan struct{})
	fact := informers.NewSharedInformerFactory(client, 0)
	fact.Certificates().V1().CertificateSigningRequests().Informer().
		AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(oldObj, newObj interface{}) {
				if csr, ok := newObj.(*v1.CertificateSigningRequest); ok {
					if csr.Name == "testcsr" && csr.Status.Certificate != nil {
						err := ioutil.WriteFile(mylib.TEST_PEM_FILE, csr.Status.Certificate, 0600)
						checkerr(err)
						ch <- struct{}{}
					}
				}
			},
		})
	mylib.CreateCSRCert(client)
	fact.Start(ch)
	<-ch
}
