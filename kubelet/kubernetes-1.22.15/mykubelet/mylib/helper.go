package mylib

import v1 "k8s.io/api/core/v1"

func SetNodeLables(node *v1.Node) {
	node.Labels["beta.kubernetes.io/os"] = "jtthink_os"
	node.Labels["kubernetes.io/hostname"] = "jtthink"
	node.Labels["type"] = "agent"
}
