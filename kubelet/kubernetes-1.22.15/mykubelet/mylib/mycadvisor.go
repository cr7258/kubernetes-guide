package mylib

import (
	"github.com/google/cadvisor/events"
	cadvisorapi "github.com/google/cadvisor/info/v1"
	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"k8s.io/kubernetes/pkg/kubelet/cadvisor"
	"time"
)

type MyCAdvisor struct {
}

func (m *MyCAdvisor) Start() error {
	//TODO implement me
	return nil
}

func (m *MyCAdvisor) DockerContainer(name string, req *cadvisorapi.ContainerInfoRequest) (cadvisorapi.ContainerInfo, error) {
	//TODO implement me
	return cadvisorapi.ContainerInfo{}, nil
}

func (m *MyCAdvisor) ContainerInfo(name string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error) {
	//TODO implement me
	return &cadvisorapi.ContainerInfo{}, nil
}

func (m *MyCAdvisor) ContainerInfoV2(name string, options cadvisorapiv2.RequestOptions) (map[string]cadvisorapiv2.ContainerInfo, error) {
	//TODO implement me
	return map[string]cadvisorapiv2.ContainerInfo{}, nil
}

func (m *MyCAdvisor) GetRequestedContainersInfo(containerName string, options cadvisorapiv2.RequestOptions) (map[string]*cadvisorapi.ContainerInfo, error) {
	//TODO implement me
	//panic("implement me GetRequestedContainersInfo")
	return map[string]*cadvisorapi.ContainerInfo{}, nil
}

func (m *MyCAdvisor) SubcontainerInfo(name string, req *cadvisorapi.ContainerInfoRequest) (map[string]*cadvisorapi.ContainerInfo, error) {
	//TODO implement me
	return map[string]*cadvisorapi.ContainerInfo{}, nil
}

func (m MyCAdvisor) MachineInfo() (*cadvisorapi.MachineInfo, error) {
	return &cadvisorapi.MachineInfo{
		Timestamp:        time.Now(),
		NumCores:         100,
		NumPhysicalCores: 100,
		NumSockets:       100,
		MemoryCapacity:   1024 * 1024 * 1024 * 32,
	}, nil
}

func (m MyCAdvisor) VersionInfo() (*cadvisorapi.VersionInfo, error) {
	return &cadvisorapi.VersionInfo{
		KernelVersion: "3.10",
	}, nil
}

func (m MyCAdvisor) ImagesFsInfo() (cadvisorapiv2.FsInfo, error) {
	return cadvisorapiv2.FsInfo{
		Timestamp: time.Now(),
		Device:    "jtthink_Device",
		Capacity:  1024 * 1024 * 1024 * 32,
		Available: 1024 * 1024 * 1024 * 32,
		Usage:     1024 * 1024 * 1024 * 16,
	}, nil
}

func (m MyCAdvisor) RootFsInfo() (cadvisorapiv2.FsInfo, error) {
	return cadvisorapiv2.FsInfo{}, nil
}

func (m MyCAdvisor) WatchEvents(request *events.Request) (*events.EventChannel, error) {
	//TODO implement me
	panic("implement me")
}

func (m MyCAdvisor) GetDirFsInfo(path string) (cadvisorapiv2.FsInfo, error) {
	return cadvisorapiv2.FsInfo{}, nil
}

var _ cadvisor.Interface = &MyCAdvisor{}
