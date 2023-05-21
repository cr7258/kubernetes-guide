package node

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"mykubelet/pkg/util"
	"runtime"
)

// 创建 Node
func RegisterNode(nodeName string, client *kubernetes.Clientset) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				v1.LabelHostname:   nodeName,
				v1.LabelOSStable:   runtime.GOOS,
				v1.LabelArchStable: runtime.GOARCH,
			},
		},
		Spec: v1.NodeSpec{},
	}
	ctx := context.TODO()

	get_node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil || get_node == nil {
		createdNode, err := client.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		if err != nil {
			klog.Fatalln(err)
		}
		klog.Infof("create node %s success \n", nodeName)
		get_node = createdNode
	}
	newNode := get_node.DeepCopy()
	setNodeStatus(newNode)

	patchBytes, err := util.PreparePatchBytesforNodeStatus(types.NodeName(nodeName), get_node, newNode)
	if err != nil {
		klog.Fatalln(err)
	}
	patchNode, err := client.CoreV1().Nodes().Patch(context.TODO(),
		nodeName, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{}, "status")
	if err != nil {
		klog.Fatalln(err)
	}
	klog.Infoln("  node status update success \n")
	get_node = patchNode
}
