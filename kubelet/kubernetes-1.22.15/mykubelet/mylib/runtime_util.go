package mylib

import (
	"encoding/json"
	v1 "k8s.io/api/core/v1"
	kubetypes "k8s.io/apimachinery/pkg/types"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"strconv"
)

type labeledContainerInfo struct {
	ContainerName string
	PodName       string
	PodNamespace  string
	PodUID        kubetypes.UID
}

func getStringValueFromLabel(labels map[string]string, label string) string {
	if value, found := labels[label]; found {
		return value
	}
	// Do not report error, because there should be many old containers without label now.
	klog.V(3).InfoS("Container doesn't have requested label, it may be an old or invalid container", "label", label)
	// Return empty string "" for these containers, the caller will get value by other ways.
	return ""
}

const (
	KubernetesPodNameLabel         = "io.kubernetes.pod.name"
	KubernetesPodNamespaceLabel    = "io.kubernetes.pod.namespace"
	KubernetesPodUIDLabel          = "io.kubernetes.pod.uid"
	KubernetesContainerNameLabel   = "io.kubernetes.container.name"
	podDeletionGracePeriodLabel    = "io.kubernetes.pod.deletionGracePeriod"
	podTerminationGracePeriodLabel = "io.kubernetes.pod.terminationGracePeriod"

	containerHashLabel                     = "io.kubernetes.container.hash"
	containerRestartCountLabel             = "io.kubernetes.container.restartCount"
	containerTerminationMessagePathLabel   = "io.kubernetes.container.terminationMessagePath"
	containerTerminationMessagePolicyLabel = "io.kubernetes.container.terminationMessagePolicy"
	containerPreStopHandlerLabel           = "io.kubernetes.container.preStopHandler"
	containerPortsLabel                    = "io.kubernetes.container.ports"

	// TODO: remove this annotation when moving to beta for Windows hostprocess containers
	// xref: https://github.com/kubernetes/kubernetes/pull/99576/commits/42fb66073214eed6fe43fa8b1586f396e30e73e3#r635392090
	// Currently, ContainerD on Windows does not yet fully support HostProcess containers
	// but will pass annotations to hcsshim which does have support.
	windowsHostProcessContainer = "microsoft.com/hostprocess-container"
)

type annotatedContainerInfo struct {
	Hash                      uint64
	RestartCount              int
	PodDeletionGracePeriod    *int64
	PodTerminationGracePeriod *int64
	TerminationMessagePath    string
	TerminationMessagePolicy  v1.TerminationMessagePolicy
	PreStopHandler            *v1.Handler
	ContainerPorts            []v1.ContainerPort
}
type containerKillReason string

const (
	reasonStartupProbe        containerKillReason = "StartupProbe"
	reasonLivenessProbe       containerKillReason = "LivenessProbe"
	reasonFailedPostStartHook containerKillReason = "FailedPostStartHook"
	reasonUnknown             containerKillReason = "Unknown"
)

type containerToKillInfo struct {
	// The spec of the container.
	container *v1.Container
	// The name of the container.
	name string
	// The message indicates why the container will be killed.
	message string
	// The reason is a clearer source of info on why a container will be killed
	// TODO: replace message with reason?
	reason containerKillReason
}
type podActions struct {
	// Stop all running (regular, init and ephemeral) containers and the sandbox for the pod.
	KillPod bool
	// Whether need to create a new sandbox. If needed to kill pod and create
	// a new pod sandbox, all init containers need to be purged (i.e., removed).
	CreateSandbox bool
	// The id of existing sandbox. It is used for starting containers in ContainersToStart.
	SandboxID string
	// The attempt number of creating sandboxes for the pod.
	Attempt uint32

	// The next init container to start.
	NextInitContainerToStart *v1.Container
	// ContainersToStart keeps a list of indexes for the containers to start,
	// where the index is the index of the specific container in the pod spec (
	// pod.Spec.Containers.
	ContainersToStart []int
	// ContainersToKill keeps a map of containers that need to be killed, note that
	// the key is the container ID of the container, while
	// the value contains necessary information to kill a container.
	ContainersToKill map[kubecontainer.ContainerID]containerToKillInfo
	// EphemeralContainersToStart is a list of indexes for the ephemeral containers to start,
	// where the index is the index of the specific container in pod.Spec.EphemeralContainers.
	EphemeralContainersToStart []int
}

func toKubeContainerState(state runtimeapi.ContainerState) kubecontainer.State {
	switch state {
	case runtimeapi.ContainerState_CONTAINER_CREATED:
		return kubecontainer.ContainerStateCreated
	case runtimeapi.ContainerState_CONTAINER_RUNNING:
		return kubecontainer.ContainerStateRunning
	case runtimeapi.ContainerState_CONTAINER_EXITED:
		return kubecontainer.ContainerStateExited
	case runtimeapi.ContainerState_CONTAINER_UNKNOWN:
		return kubecontainer.ContainerStateUnknown
	}

	return kubecontainer.ContainerStateUnknown
}

func getJSONObjectFromLabel(labels map[string]string, label string, value interface{}) (bool, error) {
	if strValue, found := labels[label]; found {
		err := json.Unmarshal([]byte(strValue), value)
		return found, err
	}
	// If the label is not found, return not found.
	return false, nil
}

func getContainerInfoFromLabels(labels map[string]string) *labeledContainerInfo {
	return &labeledContainerInfo{
		PodName:       getStringValueFromLabel(labels, KubernetesPodNameLabel),
		PodNamespace:  getStringValueFromLabel(labels, KubernetesPodNamespaceLabel),
		PodUID:        kubetypes.UID(getStringValueFromLabel(labels, KubernetesPodUIDLabel)),
		ContainerName: getStringValueFromLabel(labels, KubernetesContainerNameLabel),
	}
}
func getUint64ValueFromLabel(labels map[string]string, label string) (uint64, error) {
	if strValue, found := labels[label]; found {
		intValue, err := strconv.ParseUint(strValue, 16, 64)
		if err != nil {
			// This really should not happen. Just set value to 0 to handle this abnormal case
			return 0, err
		}
		return intValue, nil
	}
	// Do not report error, because there should be many old containers without label now.
	klog.V(3).InfoS("Container doesn't have requested label, it may be an old or invalid container", "label", label)
	// Just set the value to 0
	return 0, nil
}
func getIntValueFromLabel(labels map[string]string, label string) (int, error) {
	if strValue, found := labels[label]; found {
		intValue, err := strconv.Atoi(strValue)
		if err != nil {
			// This really should not happen. Just set value to 0 to handle this abnormal case
			return 0, err
		}
		return intValue, nil
	}
	// Do not report error, because there should be many old containers without label now.
	klog.V(3).InfoS("Container doesn't have requested label, it may be an old or invalid container", "label", label)
	// Just set the value to 0
	return 0, nil
}

func getInt64PointerFromLabel(labels map[string]string, label string) (*int64, error) {
	if strValue, found := labels[label]; found {
		int64Value, err := strconv.ParseInt(strValue, 10, 64)
		if err != nil {
			return nil, err
		}
		return &int64Value, nil
	}
	// If the label is not found, return pointer nil.
	return nil, nil
}

func getContainerInfoFromAnnotations(annotations map[string]string) *annotatedContainerInfo {
	var err error
	containerInfo := &annotatedContainerInfo{
		TerminationMessagePath:   getStringValueFromLabel(annotations, containerTerminationMessagePathLabel),
		TerminationMessagePolicy: v1.TerminationMessagePolicy(getStringValueFromLabel(annotations, containerTerminationMessagePolicyLabel)),
	}

	if containerInfo.Hash, err = getUint64ValueFromLabel(annotations, containerHashLabel); err != nil {
		klog.ErrorS(err, "Unable to get label value from annotations", "label", containerHashLabel, "annotations", annotations)
	}
	if containerInfo.RestartCount, err = getIntValueFromLabel(annotations, containerRestartCountLabel); err != nil {
		klog.ErrorS(err, "Unable to get label value from annotations", "label", containerRestartCountLabel, "annotations", annotations)
	}
	if containerInfo.PodDeletionGracePeriod, err = getInt64PointerFromLabel(annotations, podDeletionGracePeriodLabel); err != nil {
		klog.ErrorS(err, "Unable to get label value from annotations", "label", podDeletionGracePeriodLabel, "annotations", annotations)
	}
	if containerInfo.PodTerminationGracePeriod, err = getInt64PointerFromLabel(annotations, podTerminationGracePeriodLabel); err != nil {
		klog.ErrorS(err, "Unable to get label value from annotations", "label", podTerminationGracePeriodLabel, "annotations", annotations)
	}

	preStopHandler := &v1.Handler{}
	if found, err := getJSONObjectFromLabel(annotations, containerPreStopHandlerLabel, preStopHandler); err != nil {
		klog.ErrorS(err, "Unable to get label value from annotations", "label", containerPreStopHandlerLabel, "annotations", annotations)
	} else if found {
		containerInfo.PreStopHandler = preStopHandler
	}

	containerPorts := []v1.ContainerPort{}
	if found, err := getJSONObjectFromLabel(annotations, containerPortsLabel, &containerPorts); err != nil {
		klog.ErrorS(err, "Unable to get label value from annotations", "label", containerPortsLabel, "annotations", annotations)
	} else if found {
		containerInfo.ContainerPorts = containerPorts
	}

	return containerInfo
}
