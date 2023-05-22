package mylib

import (
	cri "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"time"
)

type MyRuntimeService struct {
}

func (m MyRuntimeService) Version(apiVersion string) (*runtimeapi.VersionResponse, error) {
	//TODO implement me
	return &runtimeapi.VersionResponse{
		Version:     "0.1.0", // 0.1.0
		RuntimeName: "jtthink",
	}, nil
}

func (m MyRuntimeService) CreateContainer(podSandboxID string, config *runtimeapi.ContainerConfig, sandboxConfig *runtimeapi.PodSandboxConfig) (string, error) {
	//TODO implement me
	return "", nil
}

func (m MyRuntimeService) StartContainer(containerID string) error {
	//TODO implement me
	return nil
}

func (m MyRuntimeService) StopContainer(containerID string, timeout int64) error {
	//TODO implement me
	return nil
}

func (m MyRuntimeService) RemoveContainer(containerID string) error {
	//TODO implement me
	return nil
}

func (m MyRuntimeService) ListContainers(filter *runtimeapi.ContainerFilter) ([]*runtimeapi.Container, error) {
	return MockContainers(), nil
}

func (m MyRuntimeService) ContainerStatus(containerID string) (*runtimeapi.ContainerStatus, error) {
	//TODO implement me
	return &runtimeapi.ContainerStatus{}, nil
}

func (m MyRuntimeService) UpdateContainerResources(containerID string, resources *runtimeapi.LinuxContainerResources) error {
	//TODO implement me
	panic("implement me UpdateContainerResources")
}

func (m MyRuntimeService) ExecSync(containerID string, cmd []string, timeout time.Duration) (stdout []byte, stderr []byte, err error) {
	//TODO implement me
	panic("implement me ExecSync")
}

func (m MyRuntimeService) Exec(request *runtimeapi.ExecRequest) (*runtimeapi.ExecResponse, error) {
	//TODO implement me
	panic("implement me Exec")
}

func (m MyRuntimeService) Attach(req *runtimeapi.AttachRequest) (*runtimeapi.AttachResponse, error) {
	//TODO implement me
	panic("implement me Attach")
}

func (m MyRuntimeService) ReopenContainerLog(ContainerID string) error {
	//TODO implement me
	panic("implement me ReopenContainerLog")
}

func (m MyRuntimeService) RunPodSandbox(config *runtimeapi.PodSandboxConfig, runtimeHandler string) (string, error) {
	//TODO implement me
	return "", nil
}

func (m MyRuntimeService) StopPodSandbox(podSandboxID string) error {
	//TODO implement me
	return nil
}

func (m MyRuntimeService) RemovePodSandbox(podSandboxID string) error {
	//TODO implement me
	return nil
}

func (m MyRuntimeService) PodSandboxStatus(podSandboxID string) (*runtimeapi.PodSandboxStatus, error) {
	//TODO implement me
	return &runtimeapi.PodSandboxStatus{}, nil
}

func (m MyRuntimeService) ListPodSandbox(filter *runtimeapi.PodSandboxFilter) ([]*runtimeapi.PodSandbox, error) {
	return MockSandbox(), nil
}

func (m MyRuntimeService) PortForward(request *runtimeapi.PortForwardRequest) (*runtimeapi.PortForwardResponse, error) {
	//TODO implement me
	return &runtimeapi.PortForwardResponse{}, nil
}

func (m MyRuntimeService) ContainerStats(containerID string) (*runtimeapi.ContainerStats, error) {
	//TODO implement me
	return &runtimeapi.ContainerStats{}, nil
}

func (m MyRuntimeService) ListContainerStats(filter *runtimeapi.ContainerStatsFilter) ([]*runtimeapi.ContainerStats, error) {
	//TODO implement me
	return []*runtimeapi.ContainerStats{}, nil
}

func (m MyRuntimeService) UpdateRuntimeConfig(runtimeConfig *runtimeapi.RuntimeConfig) error {
	//TODO implement me
	return nil
}

func (m MyRuntimeService) Status() (*runtimeapi.RuntimeStatus, error) {
	return &runtimeapi.RuntimeStatus{
		Conditions: []*runtimeapi.RuntimeCondition{ //必须要加这个。 否则认为容器运行时还没启动
			{
				Type:   "RuntimeReady",
				Status: true,
			},
			{
				Type:   "NetworkReady",
				Status: true,
			},
		},
	}, nil
}

var _ cri.RuntimeService = &MyRuntimeService{}
