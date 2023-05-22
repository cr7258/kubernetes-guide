package mylib

import (
	"context"
	"fmt"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	kubetypes "k8s.io/apimachinery/pkg/types"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	cri "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

type ContainerRuntime struct {
	runtimeService cri.RuntimeService
	runtimeName    string
	recorder       record.EventRecorder
}

func NewContianerRuntime(runtimeService cri.RuntimeService, runtimeName string) *ContainerRuntime {
	return &ContainerRuntime{runtimeService: runtimeService, runtimeName: runtimeName}
}

func (c ContainerRuntime) Type() string {
	return c.runtimeName
}

func (c ContainerRuntime) SupportsSingleFileMapping() bool {
	return true
}

func newRuntimeVersion(version string) (*utilversion.Version, error) {
	if ver, err := utilversion.ParseSemantic(version); err == nil {
		return ver, err
	}
	return utilversion.ParseGeneric(version)
}
func (c *ContainerRuntime) Version() (kubecontainer.Version, error) {
	//TODO implement me
	typedVersion, err := c.runtimeService.Version("")
	if err != nil {
		return nil, fmt.Errorf("get remote runtime typed version failed: %v", err)
	}
	return newRuntimeVersion(typedVersion.RuntimeVersion)
}
func (c ContainerRuntime) APIVersion() (kubecontainer.Version, error) {
	return newRuntimeVersion("0.1.0")
}

func toKubeRuntimeStatus(status *runtimeapi.RuntimeStatus) *kubecontainer.RuntimeStatus {
	conditions := []kubecontainer.RuntimeCondition{}
	for _, c := range status.GetConditions() {
		conditions = append(conditions, kubecontainer.RuntimeCondition{
			Type:    kubecontainer.RuntimeConditionType(c.Type),
			Status:  c.Status,
			Reason:  c.Reason,
			Message: c.Message,
		})
	}
	return &kubecontainer.RuntimeStatus{Conditions: conditions}
}

func (c ContainerRuntime) Status() (*kubecontainer.RuntimeStatus, error) {
	status, err := c.runtimeService.Status()
	if err != nil {
		return nil, err
	}
	return toKubeRuntimeStatus(status), nil
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
func (c *ContainerRuntime) toKubeContainer(rc *runtimeapi.Container) (*kubecontainer.Container, error) {
	if rc == nil || rc.Id == "" || rc.Image == nil {
		return nil, fmt.Errorf("unable to convert a nil pointer to a runtime container")
	}

	annotatedInfo := getContainerInfoFromAnnotations(rc.Annotations)
	return &kubecontainer.Container{
		ID:      kubecontainer.ContainerID{Type: c.runtimeName, ID: rc.Id},
		Name:    rc.GetMetadata().GetName(),
		ImageID: rc.ImageRef,
		Image:   rc.Image.Image,
		Hash:    annotatedInfo.Hash,
		State:   toKubeContainerState(rc.State),
	}, nil
}
func (c *ContainerRuntime) GetPods(all bool) ([]*kubecontainer.Pod, error) {
	pods := make(map[kubetypes.UID]*kubecontainer.Pod)
	sandboxes, err := c.getKubeletSandboxes(all)
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
		converted, err := c.sandboxToKubeContainer(s)
		if err != nil {
			klog.V(4).InfoS("Convert sandbox of pod failed", "runtimeName", c.runtimeName, "sandbox", s, "podUID", podUID, "err", err)
			continue
		}
		p.Sandboxes = append(p.Sandboxes, converted)
	}

	containers, err := c.getKubeletContainers(all)
	if err != nil {
		return nil, err
	}
	for i := range containers {
		container := containers[i]
		if container.Metadata == nil {
			klog.V(4).InfoS("Container does not have metadata", "container", c)
			continue
		}

		labelledInfo := getContainerInfoFromLabels(container.Labels)
		pod, found := pods[labelledInfo.PodUID]
		if !found {
			pod = &kubecontainer.Pod{
				ID:        labelledInfo.PodUID,
				Name:      labelledInfo.PodName,
				Namespace: labelledInfo.PodNamespace,
			}
			pods[labelledInfo.PodUID] = pod
		}

		converted, err := c.toKubeContainer(container)
		if err != nil {
			klog.V(4).InfoS("Convert container of pod failed", "runtimeName", c.runtimeName, "container", c, "podUID", labelledInfo.PodUID, "err", err)
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

func (c ContainerRuntime) GarbageCollect(gcPolicy kubecontainer.GCPolicy, allSourcesReady bool, evictNonDeletedPods bool) error {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) SyncPod(pod *v1.Pod, podStatus *kubecontainer.PodStatus, pullSecrets []v1.Secret, backOff *flowcontrol.Backoff) kubecontainer.PodSyncResult {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) KillPod(pod *v1.Pod, runningPod kubecontainer.Pod, gracePeriodOverride *int64) error {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) GetPodStatus(uid types.UID, name, namespace string) (*kubecontainer.PodStatus, error) {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) GetContainerLogs(ctx context.Context, pod *v1.Pod, containerID kubecontainer.ContainerID, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) (err error) {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) DeleteContainer(containerID kubecontainer.ContainerID) error {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) PullImage(image kubecontainer.ImageSpec, pullSecrets []v1.Secret, podSandboxConfig *runtimeapi.PodSandboxConfig) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) GetImageRef(image kubecontainer.ImageSpec) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) ListImages() ([]kubecontainer.Image, error) {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) RemoveImage(image kubecontainer.ImageSpec) error {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) ImageStats() (*kubecontainer.ImageStats, error) {
	//TODO implement me
	panic("implement me")
}

func (c ContainerRuntime) UpdatePodCIDR(podCIDR string) error {
	//TODO implement me
	panic("implement me")
}

var _ kubecontainer.Runtime = &ContainerRuntime{}
