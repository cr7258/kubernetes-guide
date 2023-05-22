package mylib

import runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

// ID 譬如 是这种 926f1b5a1d33a
// 模拟创建 沙箱
func MockSandbox() []*runtimeapi.PodSandbox {
	mypod := &runtimeapi.PodSandbox{
		Id: "926f1b5a1d33a",
		Metadata: &runtimeapi.PodSandboxMetadata{
			Name:      "mypod",
			Namespace: "default",
			Uid:       "ef14133d-c5af-482d-a514-e6fc98093553",
		},
		State: runtimeapi.PodSandboxState_SANDBOX_READY,
	}
	myngx := &runtimeapi.PodSandbox{
		Id: "726f1b3a1d32a",
		Metadata: &runtimeapi.PodSandboxMetadata{
			Name:      "myngx",
			Namespace: "default",
			Uid:       "1f14133d-r2af-482d-b524-e6gc98093321",
		},
		State: runtimeapi.PodSandboxState_SANDBOX_READY,
	}
	return []*runtimeapi.PodSandbox{mypod, myngx}
}

// 模拟创建 容器
func MockContainers() []*runtimeapi.Container {

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
		Id: "b142f836dcb9c3bbcdf6c42c04c8177bbadd30756f2d0bde1cb38086f112596d",
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
		Id: "a152f936dcb9b3bbcdf6c42c04c8177bbadd60756f2d0bde1cb38086f112596d",
		Image: &runtimeapi.ImageSpec{
			Image: "docker.io/nginx:1.18-alpine",
		},
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
		Id: "k152f936dcb9b5bbcdf6c42c04c8177bbadd60756f2d0bde1cb38086f112596d",
		Image: &runtimeapi.ImageSpec{
			Image: "docker.io/envoy-alpine",
		},
	}
	return []*runtimeapi.Container{container_mypod, container_myngx, container_myngx_sidecar}
}
