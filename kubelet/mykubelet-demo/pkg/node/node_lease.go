package node

import (
	"context"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/component-helpers/apimachinery/lease"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"os"
	"time"
)

// SetNodeOwnerFunc helps construct a newLeasePostProcessFunc which sets
// a node OwnerReference to the given lease object
func SetNodeOwnerFunc(c clientset.Interface, nodeName string) func(lease *coordinationv1.Lease) error {
	return func(lease *coordinationv1.Lease) error {
		// Setting owner reference needs node's UID. Note that it is different from
		// kubelet.nodeRef.UID. When lease is initially created, it is possible that
		// the connection between master and node is not ready yet. So try to set
		// owner reference every time when renewing the lease, until successful.
		if len(lease.OwnerReferences) == 0 {
			if node, err := c.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{}); err == nil {
				lease.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
						Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
						Name:       nodeName,
						UID:        node.UID,
					},
				}
			} else {
				klog.ErrorS(err, "Failed to get node when trying to set owner ref to the node lease", "node", klog.KRef("", nodeName))
				return err
			}
		}
		return nil
	}
}

const (
	LeaseDurationSeconds = 40
	LeaseNameSpace       = "kube-node-lease"
)

// 启动租约控制器
func StartLeaseController(kubeClient clientset.Interface, nodeName string) {
	myclock := clock.RealClock{}
	renewInterval := time.Duration(LeaseDurationSeconds * 0.25)

	heartbeatFailure := func() {
		//这里其实就是做点清理工作
		os.Exit(1)
	}

	klog.Infoln("starting lease controller")
	ctl := lease.NewController(myclock,
		kubeClient, nodeName, LeaseDurationSeconds,
		heartbeatFailure, renewInterval,
		nodeName, LeaseNameSpace,
		SetNodeOwnerFunc(kubeClient, nodeName))

	ctl.Run(wait.NeverStop)
}
