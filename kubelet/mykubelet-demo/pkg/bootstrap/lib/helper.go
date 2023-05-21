package lib

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	"log"
	"time"
)

func checkError(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}
func SetNodeLables(node *v1.Node) {
	node.Labels["beta.kubernetes.io/os"] = "jtthink_os"
	node.Labels["kubernetes.io/hostname"] = "jtthink"
	node.Labels["type"] = "agent"
}
func DurationToExpirationSeconds(duration time.Duration) *int32 {
	return pointer.Int32(int32(duration / time.Second))
}
