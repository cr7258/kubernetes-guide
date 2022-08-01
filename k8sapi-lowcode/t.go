package main

import (
	"fmt"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"k8sapi-lowcode/pkg/config"
	"log"
)

func main() {
	//k8scfg := &config.K8sConfig{} // 实际运行是注入的
	//mapper := k8scfg.RestMapper() // 实际运行是注入的
	//
	//posJson := utils.MustLoadFile("./pod.json")
	//
	//err := utils.K8sApply(posJson, k8scfg.K8sRestConfig(), *mapper)
	//fmt.Println(err)

	store := config.NewEtcdConfig().InitStore()
	cli, err := store.NewClient()
	if err != nil {
		log.Fatalln(err)
	}
	defer cli.Close()
	ch := store.Watch(cli, "/chengzw")
	for rsp := range ch {
		if err := rsp.Err(); err != nil {
			log.Println(err)
			break
		}
		stop := false
		for _, e := range rsp.Events {
			if e.Type == mvccpb.DELETE {
				fmt.Println("key 被删除")
				stop = true
			}
			if e.Type == mvccpb.PUT {
				fmt.Println("key 被修改或新增")
			}
		}
		if stop {
			break
		}
	}
}
