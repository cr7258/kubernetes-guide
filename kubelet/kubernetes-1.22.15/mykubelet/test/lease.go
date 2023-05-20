package main

import (
	"context"
	"encoding/json"
	"fmt"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"time"
)

/**
* @description 模拟租约更新，让不存在的节点状态变为 Ready
* @author chengzw
* @since 2023/5/20
* @link
 */

type Value struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type Cond struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value Value  `json:"value"`
}

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

const (
	LeaseNameSpace = "kube-node-lease"
	LeaseName      = "myjtthink"
)

// 全局 Lease
var lease *coordinationv1.Lease

// 模拟续期
func renew(client *clientset.Clientset) error {
	now := v1.NewMicroTime(time.Now())
	lease.Spec.RenewTime = &now
	newLease, err := client.CoordinationV1().Leases(LeaseNameSpace).Update(context.Background(), lease, v1.UpdateOptions{})
	checkError(err)
	lease = newLease
	return nil
}

func main() {
	restConfig, err := clientcmd.BuildConfigFromFlags("", "../kubelet.config")
	checkError(err)
	client, err := clientset.NewForConfig(restConfig)
	checkError(err)

	// 得到 Lease
	getLease, err := client.CoordinationV1().Leases(LeaseNameSpace).Get(context.Background(), LeaseName, v1.GetOptions{})
	checkError(err)
	lease = getLease
	leaseDuration := time.Duration(40) * time.Second
	renewInterval := time.Duration(float64(leaseDuration) * 0.25)

	go func() {
		for {
			err := renew(client)
			if err != nil {
				fmt.Println("renew出错:", err)
				break
			}
			time.Sleep(renewInterval)
		}
	}()

	// 更新节点状态
	payload := []Cond{
		{
			Op:   "replace",
			Path: "/status/conditions/3",
			Value: Value{
				Type:   "Ready",
				Status: "True",
			},
		},
	}
	playloadBytes, _ := json.Marshal(payload)
	node, err := client.CoreV1().Nodes().Patch(context.TODO(), "myjtthink",
		types.JSONPatchType, playloadBytes, v1.PatchOptions{}, "status",
	)
	checkError(err)
	fmt.Println(node.Status.Conditions[3])
	select {}
}
