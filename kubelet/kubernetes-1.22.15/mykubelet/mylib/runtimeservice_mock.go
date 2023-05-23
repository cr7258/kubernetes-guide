package mylib

import (
	"k8s.io/apimachinery/pkg/types"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

// 2个pod
// 第一个是mypod  --- 容器是mypod-container
// 第二个是myngx  -- 容器有2个  myngx-container  和 myngx_sidecar-container
// 弄成全局的，方便 从外部修改
var MockData_Pods = []*runtimeapi.PodSandbox{}
var MockData_Containers = []*runtimeapi.Container{}

var (
	//假货 mypod 的ID
	MockPod_MyPod_Id               = "926f1b5a1d33a"
	MockPod_MyNgx_Id               = "726f1b3a1d32a"
	MockContainer_MyPod_Id         = "b142f836dcb9c3bb"
	MockContainer_MyNgx_Id         = "a152f936dc"
	MockContainer_MyNgx_SideCar_Id = "k152f936dcb"
	//这个假数据是给 cri 用的  ,默认都是 ready
	MockPod_CRI_Status = map[string]runtimeapi.PodSandboxState{
		MockPod_MyPod_Id: runtimeapi.PodSandboxState_SANDBOX_READY,
		MockPod_MyNgx_Id: runtimeapi.PodSandboxState_SANDBOX_READY,
	}
	MockContainer_CRI_Status = map[string]kubecontainer.State{

		MockContainer_MyPod_Id:         kubecontainer.ContainerStateRunning,
		MockContainer_MyNgx_Id:         kubecontainer.ContainerStateRunning,
		MockContainer_MyNgx_SideCar_Id: kubecontainer.ContainerStateRunning,
	}
)

func init() {
	mypod := &runtimeapi.PodSandbox{
		Id: MockPod_MyPod_Id,
		Metadata: &runtimeapi.PodSandboxMetadata{
			Name:      "mypod",
			Namespace: "default",
			Uid:       "ef14133d-c5af-482d-a514-e6fc98093553",
		},
		Annotations: map[string]string{
			"app": "mypod",
		},
		State: runtimeapi.PodSandboxState_SANDBOX_READY,
	}
	myngx := &runtimeapi.PodSandbox{
		Id: MockPod_MyNgx_Id,
		Metadata: &runtimeapi.PodSandboxMetadata{
			Name:      "myngx",
			Namespace: "default",
			Uid:       "1f14133d-r2af-482d-b524-e6gc98093321",
		},
		Annotations: map[string]string{
			"app": "myngx",
		},
		State: runtimeapi.PodSandboxState_SANDBOX_READY,
	}
	MockData_Pods = []*runtimeapi.PodSandbox{mypod, myngx}

	container_mypod := &runtimeapi.Container{
		Metadata: &runtimeapi.ContainerMetadata{
			Name: "mypod-container",
		},
		Labels: map[string]string{
			KubernetesPodNameLabel:       "mypod",
			KubernetesPodNamespaceLabel:  "default", //默认 设置在default
			KubernetesPodUIDLabel:        "ef14133d-c5af-482d-a514-e6fc98093553",
			KubernetesContainerNameLabel: "mypod-container",
		},
		Id: MockContainer_MyPod_Id,
		Image: &runtimeapi.ImageSpec{
			Image: "jtthink.com/mockimage@latest",
		},
	}

	container_myngx := &runtimeapi.Container{
		Metadata: &runtimeapi.ContainerMetadata{
			Name: "myngx-container",
		},
		Labels: map[string]string{
			KubernetesPodNameLabel:       "myngx",
			KubernetesPodNamespaceLabel:  "default", //默认 设置在default
			KubernetesPodUIDLabel:        "1f14133d-r2af-482d-b524-e6gc98093321",
			KubernetesContainerNameLabel: "myngx-container",
		},
		Id: MockContainer_MyNgx_Id,
		Image: &runtimeapi.ImageSpec{
			Image: "docker.io/nginx:1.18-alpine",
		},
		State: runtimeapi.ContainerState_CONTAINER_RUNNING,
	}

	container_myngx_sidecar := &runtimeapi.Container{
		Metadata: &runtimeapi.ContainerMetadata{
			Name: "myngx_sidecar-container",
		},
		Labels: map[string]string{
			KubernetesPodNameLabel:       "myngx",
			KubernetesPodNamespaceLabel:  "default", //默认 设置在default
			KubernetesPodUIDLabel:        "1f14133d-r2af-482d-b524-e6gc98093321",
			KubernetesContainerNameLabel: "myngx_sidecar-container",
		},
		Id: MockContainer_MyNgx_SideCar_Id,
		Image: &runtimeapi.ImageSpec{
			Image: "docker.io/envoy-alpine",
		},
		//默认 一开始是running
		State: runtimeapi.ContainerState_CONTAINER_RUNNING,
	}

	MockData_Containers = []*runtimeapi.Container{container_mypod, container_myngx, container_myngx_sidecar}

}

// 根据UID 找到POD
func findPodIDByUID(uid types.UID) string {
	for _, p := range MockData_Pods {
		if p.Metadata.Uid == string(uid) {
			return p.Id
		}
	}
	return ""
}
func findContainerIdsByPodUID(uid types.UID) []string {
	c := []string{}
	for _, container := range MockData_Containers {
		if container.Labels[KubernetesPodUIDLabel] == string(uid) {
			c = append(c, container.Id)
		}
	}
	return c
}

// ID 譬如 是这种 926f1b5a1d33a
// 模拟创建 沙箱
func MockSandbox() []*runtimeapi.PodSandbox {

	return MockData_Pods //全局变量
}

// 模拟创建 容器
func MockContainers() []*runtimeapi.Container {

	return MockData_Containers
}
