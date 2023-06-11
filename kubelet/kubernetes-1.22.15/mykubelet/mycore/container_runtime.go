package mycore

import (
	"context"
	"fmt"
	"io"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubetypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/flowcontrol"
	cri "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	crierror "k8s.io/cri-api/pkg/errors"
	"k8s.io/klog/v2"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"net"
	"sort"
)

const (
	RuntimeVersion    = "0.1.0"
	RuntimeApiVersion = "v1"
)

type ContainerRuntime struct {
	runtimeService cri.RuntimeService
	runtimeName    string
}

func NewContainerRuntime(rt cri.RuntimeService) *ContainerRuntime {
	return &ContainerRuntime{
		runtimeService: rt, runtimeName: "myruntime",
	}
}

func (m *ContainerRuntime) SupportsSingleFileMapping() bool {
	//TODO implement me
	return true
}

func (m *ContainerRuntime) getTypedVersion() (*runtimeapi.VersionResponse, error) {
	typedVersion, err := m.runtimeService.Version(RuntimeVersion)
	if err != nil {
		return nil, fmt.Errorf("get remote runtime typed version failed: %v", err)
	}
	return typedVersion, nil
}
func (m *ContainerRuntime) Version() (kubecontainer.Version, error) {
	typedVersion, err := m.getTypedVersion()
	if err != nil {
		return nil, err
	}

	return newRuntimeVersion(typedVersion.RuntimeVersion)
}

func (m *ContainerRuntime) APIVersion() (kubecontainer.Version, error) {
	return newRuntimeVersion(RuntimeApiVersion)
}

func (m *ContainerRuntime) Status() (*kubecontainer.RuntimeStatus, error) {
	status, err := m.runtimeService.Status()
	if err != nil {
		return nil, err
	}
	return toKubeRuntimeStatus(status), nil
}

func (m *ContainerRuntime) GetPods(all bool) ([]*kubecontainer.Pod, error) {
	pods := make(map[kubetypes.UID]*kubecontainer.Pod)
	sandboxes, err := m.getKubeletSandboxes(all)
	if err != nil {
		return nil, err
	}
	for i := range sandboxes {
		s := sandboxes[i]
		if s.Metadata == nil {
			klog.V(4).InfoS("Sandbox does not have metadata", "sandbox", s)
			continue
		}
		podUID := kubetypes.UID(s.Metadata.Uid)
		if _, ok := pods[podUID]; !ok {
			pods[podUID] = &kubecontainer.Pod{
				ID:        podUID,
				Name:      s.Metadata.Name,
				Namespace: s.Metadata.Namespace,
			}
		}
		p := pods[podUID]
		converted, err := m.sandboxToKubeContainer(s)
		if err != nil {
			klog.V(4).InfoS("Convert sandbox of pod failed", "runtimeName", m.runtimeName, "sandbox", s, "podUID", podUID, "err", err)
			continue
		}
		p.Sandboxes = append(p.Sandboxes, converted)
	}

	containers, err := m.getKubeletContainers(all)
	if err != nil {
		return nil, err
	}
	for i := range containers {
		getc := containers[i]
		if getc.Metadata == nil {
			klog.V(4).InfoS("Container does not have metadata", "container", m)
			continue
		}

		labelledInfo := getContainerInfoFromLabels(getc.Labels)
		pod, found := pods[labelledInfo.PodUID]
		if !found {
			pod = &kubecontainer.Pod{
				ID:        labelledInfo.PodUID,
				Name:      labelledInfo.PodName,
				Namespace: labelledInfo.PodNamespace,
			}
			pods[labelledInfo.PodUID] = pod
		}

		converted, err := m.toKubeContainer(getc)
		if err != nil {
			klog.V(4).InfoS("Convert container of pod failed", "runtimeName", m.runtimeName, "container", m, "podUID", labelledInfo.PodUID, "err", err)
			continue
		}

		pod.Containers = append(pod.Containers, converted)
	}

	// Convert map to list.
	var result []*kubecontainer.Pod
	for _, pod := range pods {
		result = append(result, pod)
	}

	return result, nil
}

func (m *ContainerRuntime) GarbageCollect(gcPolicy kubecontainer.GCPolicy, allSourcesReady bool, evictNonDeletedPods bool) error {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) SyncPod(pod *v1.Pod, podStatus *kubecontainer.PodStatus, pullSecrets []v1.Secret, backOff *flowcontrol.Backoff) kubecontainer.PodSyncResult {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) KillPod(pod *v1.Pod, runningPod kubecontainer.Pod, gracePeriodOverride *int64) error {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) getSandboxIDByPodUID(podUID kubetypes.UID, state *runtimeapi.PodSandboxState) ([]string, error) {
	filter := &runtimeapi.PodSandboxFilter{
		LabelSelector: map[string]string{KubernetesPodUIDLabel: string(podUID)},
	}
	if state != nil {
		filter.State = &runtimeapi.PodSandboxStateValue{
			State: *state,
		}
	}
	sandboxes, err := m.runtimeService.ListPodSandbox(filter)
	if err != nil {
		klog.ErrorS(err, "Failed to list sandboxes for pod", "podUID", podUID)
		return nil, err
	}

	if len(sandboxes) == 0 {
		return nil, nil
	}

	// Sort with newest first.
	sandboxIDs := make([]string, len(sandboxes))
	sort.Sort(podSandboxByCreated(sandboxes))
	for i, s := range sandboxes {
		sandboxIDs[i] = s.Id
	}

	return sandboxIDs, nil
}
func (m *ContainerRuntime) determinePodSandboxIPs(podNamespace, podName string, podSandbox *runtimeapi.PodSandboxStatus) []string {
	podIPs := make([]string, 0)
	if podSandbox.Network == nil {
		klog.InfoS("Pod Sandbox status doesn't have network information, cannot report IPs", "pod", klog.KRef(podNamespace, podName))
		return podIPs
	}

	// ip could be an empty string if runtime is not responsible for the
	// IP (e.g., host networking).

	// pick primary IP
	if len(podSandbox.Network.Ip) != 0 {
		if net.ParseIP(podSandbox.Network.Ip) == nil {
			klog.InfoS("Pod Sandbox reported an unparseable primary IP", "pod", klog.KRef(podNamespace, podName), "IP", podSandbox.Network.Ip)
			return nil
		}
		podIPs = append(podIPs, podSandbox.Network.Ip)
	}

	// pick additional ips, if cri reported them
	for _, podIP := range podSandbox.Network.AdditionalIps {
		if nil == net.ParseIP(podIP.Ip) {
			klog.InfoS("Pod Sandbox reported an unparseable additional IP", "pod", klog.KRef(podNamespace, podName), "IP", podIP.Ip)
			return nil
		}
		podIPs = append(podIPs, podIP.Ip)
	}

	return podIPs
}
func (m *ContainerRuntime) getPodContainerStatuses(uid kubetypes.UID, name, namespace string) ([]*kubecontainer.Status, error) {
	// Select all containers of the given pod.
	containers, err := m.runtimeService.ListContainers(&runtimeapi.ContainerFilter{
		LabelSelector: map[string]string{KubernetesPodUIDLabel: string(uid)},
	})
	if err != nil {
		klog.ErrorS(err, "ListContainers error")
		return nil, err
	}

	statuses := []*kubecontainer.Status{}
	// TODO: optimization: set maximum number of containers per container name to examine.
	for _, c := range containers {
		status, err := m.runtimeService.ContainerStatus(c.Id)
		// Between List (ListContainers) and check (ContainerStatus) another thread might remove a container, and that is normal.
		// The previous call (ListContainers) never fails due to a pod container not existing.
		// Therefore, this method should not either, but instead act as if the previous call failed,
		// which means the error should be ignored.
		if crierror.IsNotFound(err) {
			continue
		}
		if err != nil {
			// Merely log this here; GetPodStatus will actually report the error out.
			klog.V(4).InfoS("ContainerStatus return error", "containerID", c.Id, "err", err)
			return nil, err
		}
		cStatus := toKubeContainerStatus(status, m.runtimeName)
		if status.State == runtimeapi.ContainerState_CONTAINER_EXITED {
			// Populate the termination message if needed.
			annotatedInfo := getContainerInfoFromAnnotations(status.Annotations)
			// If a container cannot even be started, it certainly does not have logs, so no need to fallbackToLogs.
			fallbackToLogs := annotatedInfo.TerminationMessagePolicy == v1.TerminationMessageFallbackToLogsOnError &&
				cStatus.ExitCode != 0 && cStatus.Reason != "ContainerCannotRun"
			tMessage, _ := getTerminationMessage(status, annotatedInfo.TerminationMessagePath, fallbackToLogs)

			// Enrich the termination message written by the application is not empty
			if len(tMessage) != 0 {
				if len(cStatus.Message) != 0 {
					cStatus.Message += ": "
				}
				cStatus.Message += tMessage
			}
		}
		statuses = append(statuses, cStatus)
	}

	sort.Sort(containerStatusByCreated(statuses))
	return statuses, nil
}

func (c *ContainerRuntime) GetPodStatus(uid types.UID, name, namespace string) (*kubecontainer.PodStatus, error) {
	podSandboxIDs, err := c.getSandboxIDByPodUID(uid, nil)
	if err != nil {
		return nil, err
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
	}

	//podFullName := format.Pod(pod)

	klog.V(4).InfoS("getSandboxIDByPodUID got sandbox IDs for pod", "podSandboxID", podSandboxIDs, "pod", klog.KObj(pod))

	sandboxStatuses := []*runtimeapi.PodSandboxStatus{}
	podIPs := []string{}
	for idx, podSandboxID := range podSandboxIDs {
		podSandboxStatus, err := c.runtimeService.PodSandboxStatus(podSandboxID)
		// Between List (getSandboxIDByPodUID) and check (PodSandboxStatus) another thread might remove a container, and that is normal.
		// The previous call (getSandboxIDByPodUID) never fails due to a pod sandbox not existing.
		// Therefore, this method should not either, but instead act as if the previous call failed,
		// which means the error should be ignored.
		if crierror.IsNotFound(err) {
			continue
		}
		if err != nil {
			klog.ErrorS(err, "PodSandboxStatus of sandbox for pod", "podSandboxID", podSandboxID, "pod", klog.KObj(pod))
			return nil, err
		}
		sandboxStatuses = append(sandboxStatuses, podSandboxStatus)
		// Only get pod IP from latest sandbox
		if idx == 0 && podSandboxStatus.State == runtimeapi.PodSandboxState_SANDBOX_READY {
			podIPs = c.determinePodSandboxIPs(namespace, name, podSandboxStatus)
		}
	}

	// Get statuses of all containers visible in the pod.
	containerStatuses, err := c.getPodContainerStatuses(uid, name, namespace)
	if err != nil {

		return nil, err
	}

	return &kubecontainer.PodStatus{
		ID:                uid,
		Name:              name,
		Namespace:         namespace,
		IPs:               podIPs,
		SandboxStatuses:   sandboxStatuses,
		ContainerStatuses: containerStatuses,
	}, nil
}

func (m *ContainerRuntime) GetContainerLogs(ctx context.Context, pod *v1.Pod, containerID kubecontainer.ContainerID, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) (err error) {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) DeleteContainer(containerID kubecontainer.ContainerID) error {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) PullImage(image kubecontainer.ImageSpec, pullSecrets []v1.Secret, podSandboxConfig *runtimeapi.PodSandboxConfig) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) GetImageRef(image kubecontainer.ImageSpec) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) ListImages() ([]kubecontainer.Image, error) {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) RemoveImage(image kubecontainer.ImageSpec) error {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) ImageStats() (*kubecontainer.ImageStats, error) {
	//TODO implement me
	panic("implement me")
}

func (m *ContainerRuntime) UpdatePodCIDR(podCIDR string) error {
	//TODO implement me
	panic("implement me")
}

var _ kubecontainer.Runtime = &ContainerRuntime{}

func (m *ContainerRuntime) Type() string {
	//TODO implement me
	panic("implement me")
}

func (c *ContainerRuntime) getKubeletSandboxes(all bool) ([]*runtimeapi.PodSandbox, error) {
	var filter *runtimeapi.PodSandboxFilter
	if !all {
		readyState := runtimeapi.PodSandboxState_SANDBOX_READY
		filter = &runtimeapi.PodSandboxFilter{
			State: &runtimeapi.PodSandboxStateValue{
				State: readyState,
			},
		}
	}

	resp, err := c.runtimeService.ListPodSandbox(filter)
	if err != nil {
		klog.ErrorS(err, "Failed to list pod sandboxes")
		return nil, err
	}

	return resp, nil
}
func (c *ContainerRuntime) sandboxToKubeContainer(s *runtimeapi.PodSandbox) (*kubecontainer.Container, error) {
	if s == nil || s.Id == "" {
		return nil, fmt.Errorf("unable to convert a nil pointer to a runtime container")
	}

	return &kubecontainer.Container{
		ID:    kubecontainer.ContainerID{Type: c.runtimeName, ID: s.Id},
		State: kubecontainer.SandboxToContainerState(s.State),
	}, nil
}
func (c *ContainerRuntime) getKubeletContainers(allContainers bool) ([]*runtimeapi.Container, error) {
	filter := &runtimeapi.ContainerFilter{}
	if !allContainers {
		filter.State = &runtimeapi.ContainerStateValue{
			State: runtimeapi.ContainerState_CONTAINER_RUNNING,
		}
	}

	containers, err := c.runtimeService.ListContainers(filter)
	if err != nil {
		klog.ErrorS(err, "ListContainers failed")
		return nil, err
	}

	return containers, nil
}
func (m *ContainerRuntime) toKubeContainer(c *runtimeapi.Container) (*kubecontainer.Container, error) {
	if c == nil || c.Id == "" || c.Image == nil {
		return nil, fmt.Errorf("unable to convert a nil pointer to a runtime container")
	}

	annotatedInfo := getContainerInfoFromAnnotations(c.Annotations)
	return &kubecontainer.Container{
		ID:      kubecontainer.ContainerID{Type: m.runtimeName, ID: c.Id},
		Name:    c.GetMetadata().GetName(),
		ImageID: c.ImageRef,
		Image:   c.Image.Image,
		Hash:    annotatedInfo.Hash,
		State:   toKubeContainerState(c.State),
	}, nil
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
