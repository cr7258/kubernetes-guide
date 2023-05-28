package mycore

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
)

func SyncPodFn(ctx context.Context, updateType kubetypes.SyncPodType, pod *v1.Pod, mirrorPod *v1.Pod, podStatus *kubecontainer.PodStatus) (bool, error) {
	fmt.Println("临时的syncpod函数")
	return true, nil
}
func SyncTerminatingFn(ctx context.Context, pod *v1.Pod, podStatus *kubecontainer.PodStatus, runningPod *kubecontainer.Pod, gracePeriod *int64, podStatusFn func(*v1.PodStatus)) error {
	fmt.Println("临时的SyncTerminating函数")
	return nil
}

// the function to invoke to terminate a pod (ensure no running processes are present)
func SyncTerminatedFn(ctx context.Context, pod *v1.Pod, podStatus *kubecontainer.PodStatus) error {
	fmt.Println("临时的SyncTerminated函数")
	return nil
}
