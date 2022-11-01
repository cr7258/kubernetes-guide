package main

import (
	"flag"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/clientcmd"
	"path/filepath"
	"k8s.io/client-go/util/homedir"
)

func main() {
	kubeconfigPath := flag.String("kubeconfig", filepath.Join(homedir.HomeDir(), ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfigPath)
	if err != nil {
		panic(err)
	}

	discoverClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		panic(err)
	}

	_, apiResourceList, err := discoverClient.ServerGroupsAndResources()
	for _, v := range apiResourceList {
		gv, err := schema.ParseGroupVersion(v.GroupVersion)
		if err != nil {
			panic(err)
		}
		for _, resource := range v.APIResources {
			fmt.Println("name:", resource.Name, "    ", "group:", gv.Group, "    ", "version:", gv.Version)
		}
	}
}