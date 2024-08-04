/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kuberuntime

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	runtimetesting "k8s.io/cri-api/pkg/apis/testing"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	utilpointer "k8s.io/utils/pointer"
)

func seccompLocalhostRef(profileName string) string {
	return filepath.Join(fakeSeccompProfileRoot, profileName)
}

func seccompLocalhostPath(profileName string) string {
	return "localhost/" + seccompLocalhostRef(profileName)
}

func TestIsInitContainerFailed(t *testing.T) {
	tests := []struct {
		status      *kubecontainer.Status
		isFailed    bool
		description string
	}{
		{
			status: &kubecontainer.Status{
				State:    kubecontainer.ContainerStateExited,
				ExitCode: 1,
			},
			isFailed:    true,
			description: "Init container in exited state and non-zero exit code should return true",
		},
		{
			status: &kubecontainer.Status{
				State: kubecontainer.ContainerStateUnknown,
			},
			isFailed:    true,
			description: "Init container in unknown state should return true",
		},
		{
			status: &kubecontainer.Status{
				Reason:   "OOMKilled",
				ExitCode: 0,
			},
			isFailed:    true,
			description: "Init container which reason is OOMKilled should return true",
		},
		{
			status: &kubecontainer.Status{
				State:    kubecontainer.ContainerStateExited,
				ExitCode: 0,
			},
			isFailed:    false,
			description: "Init container in exited state and zero exit code should return false",
		},
		{
			status: &kubecontainer.Status{
				State: kubecontainer.ContainerStateRunning,
			},
			isFailed:    false,
			description: "Init container in running state should return false",
		},
		{
			status: &kubecontainer.Status{
				State: kubecontainer.ContainerStateCreated,
			},
			isFailed:    false,
			description: "Init container in created state should return false",
		},
	}
	for i, test := range tests {
		isFailed := isInitContainerFailed(test.status)
		assert.Equal(t, test.isFailed, isFailed, "TestCase[%d]: %s", i, test.description)
	}
}

func TestStableKey(t *testing.T) {
	container := &v1.Container{
		Name:  "test_container",
		Image: "foo/image:v1",
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test_pod",
			Namespace: "test_pod_namespace",
			UID:       "test_pod_uid",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{*container},
		},
	}
	oldKey := getStableKey(pod, container)

	// Updating the container image should change the key.
	container.Image = "foo/image:v2"
	newKey := getStableKey(pod, container)
	assert.NotEqual(t, oldKey, newKey)
}

func TestToKubeContainer(t *testing.T) {
	c := &runtimeapi.Container{
		Id: "test-id",
		Metadata: &runtimeapi.ContainerMetadata{
			Name:    "test-name",
			Attempt: 1,
		},
		Image:    &runtimeapi.ImageSpec{Image: "test-image"},
		ImageRef: "test-image-ref",
		State:    runtimeapi.ContainerState_CONTAINER_RUNNING,
		Annotations: map[string]string{
			containerHashLabel: "1234",
		},
	}
	expect := &kubecontainer.Container{
		ID: kubecontainer.ContainerID{
			Type: runtimetesting.FakeRuntimeName,
			ID:   "test-id",
		},
		Name:    "test-name",
		ImageID: "test-image-ref",
		Image:   "test-image",
		Hash:    uint64(0x1234),
		State:   kubecontainer.ContainerStateRunning,
	}

	_, _, m, err := createTestRuntimeManager()
	assert.NoError(t, err)
	got, err := m.toKubeContainer(c)
	assert.NoError(t, err)
	assert.Equal(t, expect, got)
}

func TestGetImageUser(t *testing.T) {
	_, i, m, err := createTestRuntimeManager()
	assert.NoError(t, err)

	type image struct {
		name     string
		uid      *runtimeapi.Int64Value
		username string
	}

	type imageUserValues struct {
		// getImageUser can return (*int64)(nil) so comparing with *uid will break
		// type cannot be *int64 as Golang does not allow to take the address of a numeric constant"
		uid      interface{}
		username string
		err      error
	}

	tests := []struct {
		description             string
		originalImage           image
		expectedImageUserValues imageUserValues
	}{
		{
			"image without username and uid should return (new(int64), \"\", nil)",
			image{
				name:     "test-image-ref1",
				uid:      (*runtimeapi.Int64Value)(nil),
				username: "",
			},
			imageUserValues{
				uid:      int64(0),
				username: "",
				err:      nil,
			},
		},
		{
			"image with username and no uid should return ((*int64)nil, imageStatus.Username, nil)",
			image{
				name:     "test-image-ref2",
				uid:      (*runtimeapi.Int64Value)(nil),
				username: "testUser",
			},
			imageUserValues{
				uid:      (*int64)(nil),
				username: "testUser",
				err:      nil,
			},
		},
		{
			"image with uid should return (*int64, \"\", nil)",
			image{
				name: "test-image-ref3",
				uid: &runtimeapi.Int64Value{
					Value: 2,
				},
				username: "whatever",
			},
			imageUserValues{
				uid:      int64(2),
				username: "",
				err:      nil,
			},
		},
	}

	i.SetFakeImages([]string{"test-image-ref1", "test-image-ref2", "test-image-ref3"})
	for j, test := range tests {
		i.Images[test.originalImage.name].Username = test.originalImage.username
		i.Images[test.originalImage.name].Uid = test.originalImage.uid

		uid, username, err := m.getImageUser(test.originalImage.name)
		assert.NoError(t, err, "TestCase[%d]", j)

		if test.expectedImageUserValues.uid == (*int64)(nil) {
			assert.Equal(t, test.expectedImageUserValues.uid, uid, "TestCase[%d]", j)
		} else {
			assert.Equal(t, test.expectedImageUserValues.uid, *uid, "TestCase[%d]", j)
		}
		assert.Equal(t, test.expectedImageUserValues.username, username, "TestCase[%d]", j)
	}
}

func TestFieldProfile(t *testing.T) {
	tests := []struct {
		description     string
		scmpProfile     *v1.SeccompProfile
		rootPath        string
		expectedProfile string
	}{
		{
			description:     "no seccompProfile should return empty",
			expectedProfile: "",
		},
		{
			description: "type localhost without profile should return empty",
			scmpProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeLocalhost,
			},
			expectedProfile: "",
		},
		{
			description: "unknown type should return empty",
			scmpProfile: &v1.SeccompProfile{
				Type: "",
			},
			expectedProfile: "",
		},
		{
			description: "SeccompProfileTypeRuntimeDefault should return runtime/default",
			scmpProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeRuntimeDefault,
			},
			expectedProfile: "runtime/default",
		},
		{
			description: "SeccompProfileTypeUnconfined should return unconfined",
			scmpProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeUnconfined,
			},
			expectedProfile: "unconfined",
		},
		{
			description: "SeccompProfileTypeLocalhost should return localhost",
			scmpProfile: &v1.SeccompProfile{
				Type:             v1.SeccompProfileTypeLocalhost,
				LocalhostProfile: utilpointer.StringPtr("profile.json"),
			},
			rootPath:        "/test/",
			expectedProfile: "localhost//test/profile.json",
		},
	}

	for i, test := range tests {
		seccompProfile := fieldProfile(test.scmpProfile, test.rootPath, false)
		assert.Equal(t, test.expectedProfile, seccompProfile, "TestCase[%d]: %s", i, test.description)
	}
}

func TestFieldProfileDefaultSeccomp(t *testing.T) {
	tests := []struct {
		description     string
		scmpProfile     *v1.SeccompProfile
		rootPath        string
		expectedProfile string
	}{
		{
			description:     "no seccompProfile should return runtime/default",
			expectedProfile: v1.SeccompProfileRuntimeDefault,
		},
		{
			description: "type localhost without profile should return runtime/default",
			scmpProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeLocalhost,
			},
			expectedProfile: v1.SeccompProfileRuntimeDefault,
		},
		{
			description: "unknown type should return runtime/default",
			scmpProfile: &v1.SeccompProfile{
				Type: "",
			},
			expectedProfile: v1.SeccompProfileRuntimeDefault,
		},
		{
			description: "SeccompProfileTypeRuntimeDefault should return runtime/default",
			scmpProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeRuntimeDefault,
			},
			expectedProfile: "runtime/default",
		},
		{
			description: "SeccompProfileTypeUnconfined should return unconfined",
			scmpProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeUnconfined,
			},
			expectedProfile: "unconfined",
		},
		{
			description: "SeccompProfileTypeLocalhost should return localhost",
			scmpProfile: &v1.SeccompProfile{
				Type:             v1.SeccompProfileTypeLocalhost,
				LocalhostProfile: utilpointer.StringPtr("profile.json"),
			},
			rootPath:        "/test/",
			expectedProfile: "localhost//test/profile.json",
		},
	}

	for i, test := range tests {
		seccompProfile := fieldProfile(test.scmpProfile, test.rootPath, true)
		assert.Equal(t, test.expectedProfile, seccompProfile, "TestCase[%d]: %s", i, test.description)
	}
}

func TestGetSeccompProfilePath(t *testing.T) {
	_, _, m, err := createTestRuntimeManager()
	require.NoError(t, err)

	tests := []struct {
		description     string
		annotation      map[string]string
		podSc           *v1.PodSecurityContext
		containerSc     *v1.SecurityContext
		containerName   string
		expectedProfile string
	}{
		{
			description:     "no seccomp should return empty",
			expectedProfile: "",
		},
		{
			description:     "annotations: no seccomp with containerName should return empty",
			containerName:   "container1",
			expectedProfile: "",
		},
		{
			description:     "pod seccomp profile set to unconfined returns unconfined",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			expectedProfile: "unconfined",
		},
		{
			description:     "container seccomp profile set to unconfined returns unconfined",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			expectedProfile: "unconfined",
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeRuntimeDefault returns runtime/default",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: "runtime/default",
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeRuntimeDefault returns runtime/default",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: "runtime/default",
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeLocalhost returns 'localhost/' + LocalhostProfile",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("filename")}},
			expectedProfile: seccompLocalhostPath("filename"),
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeLocalhost with empty LocalhostProfile returns empty",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost}},
			expectedProfile: "",
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeLocalhost with empty LocalhostProfile returns empty",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost}},
			expectedProfile: "",
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeLocalhost returns 'localhost/' + LocalhostProfile",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("filename2")}},
			expectedProfile: seccompLocalhostPath("filename2"),
		},
		{
			description:     "prioritise container field over pod field",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: "runtime/default",
		},
	}

	for i, test := range tests {
		seccompProfile := m.getSeccompProfilePath(test.annotation, test.containerName, test.podSc, test.containerSc, false)
		assert.Equal(t, test.expectedProfile, seccompProfile, "TestCase[%d]: %s", i, test.description)
	}
}

func TestGetSeccompProfilePathDefaultSeccomp(t *testing.T) {
	_, _, m, err := createTestRuntimeManager()
	require.NoError(t, err)

	tests := []struct {
		description     string
		annotation      map[string]string
		podSc           *v1.PodSecurityContext
		containerSc     *v1.SecurityContext
		containerName   string
		expectedProfile string
	}{
		{
			description:     "no seccomp should return runtime/default",
			expectedProfile: v1.SeccompProfileRuntimeDefault,
		},
		{
			description:     "annotations: no seccomp with containerName should return runtime/default",
			containerName:   "container1",
			expectedProfile: v1.SeccompProfileRuntimeDefault,
		},
		{
			description:     "pod seccomp profile set to unconfined returns unconfined",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			expectedProfile: "unconfined",
		},
		{
			description:     "container seccomp profile set to unconfined returns unconfined",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			expectedProfile: "unconfined",
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeRuntimeDefault returns runtime/default",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: "runtime/default",
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeRuntimeDefault returns runtime/default",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: "runtime/default",
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeLocalhost returns 'localhost/' + LocalhostProfile",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("filename")}},
			expectedProfile: seccompLocalhostPath("filename"),
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeLocalhost with empty LocalhostProfile returns runtime/default",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost}},
			expectedProfile: v1.SeccompProfileRuntimeDefault,
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeLocalhost with empty LocalhostProfile returns runtime/default",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost}},
			expectedProfile: v1.SeccompProfileRuntimeDefault,
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeLocalhost returns 'localhost/' + LocalhostProfile",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("filename2")}},
			expectedProfile: seccompLocalhostPath("filename2"),
		},
		{
			description:     "prioritise container field over pod field",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: "runtime/default",
		},
	}

	for i, test := range tests {
		seccompProfile := m.getSeccompProfilePath(test.annotation, test.containerName, test.podSc, test.containerSc, true)
		assert.Equal(t, test.expectedProfile, seccompProfile, "TestCase[%d]: %s", i, test.description)
	}
}

func TestGetSeccompProfile(t *testing.T) {
	_, _, m, err := createTestRuntimeManager()
	require.NoError(t, err)

	unconfinedProfile := &runtimeapi.SecurityProfile{
		ProfileType: runtimeapi.SecurityProfile_Unconfined,
	}

	runtimeDefaultProfile := &runtimeapi.SecurityProfile{
		ProfileType: runtimeapi.SecurityProfile_RuntimeDefault,
	}

	tests := []struct {
		description     string
		annotation      map[string]string
		podSc           *v1.PodSecurityContext
		containerSc     *v1.SecurityContext
		containerName   string
		expectedProfile *runtimeapi.SecurityProfile
	}{
		{
			description:     "no seccomp should return unconfined",
			expectedProfile: unconfinedProfile,
		},
		{
			description:     "pod seccomp profile set to unconfined returns unconfined",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			expectedProfile: unconfinedProfile,
		},
		{
			description:     "container seccomp profile set to unconfined returns unconfined",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			expectedProfile: unconfinedProfile,
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeRuntimeDefault returns runtime/default",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: runtimeDefaultProfile,
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeRuntimeDefault returns runtime/default",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: runtimeDefaultProfile,
		},
		{
			description: "pod seccomp profile set to SeccompProfileTypeLocalhost returns 'localhost/' + LocalhostProfile",
			podSc:       &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("filename")}},
			expectedProfile: &runtimeapi.SecurityProfile{
				ProfileType:  runtimeapi.SecurityProfile_Localhost,
				LocalhostRef: seccompLocalhostRef("filename"),
			},
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeLocalhost with empty LocalhostProfile returns unconfined",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost}},
			expectedProfile: unconfinedProfile,
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeLocalhost with empty LocalhostProfile returns unconfined",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost}},
			expectedProfile: unconfinedProfile,
		},
		{
			description: "container seccomp profile set to SeccompProfileTypeLocalhost returns 'localhost/' + LocalhostProfile",
			containerSc: &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("filename2")}},
			expectedProfile: &runtimeapi.SecurityProfile{
				ProfileType:  runtimeapi.SecurityProfile_Localhost,
				LocalhostRef: seccompLocalhostRef("filename2"),
			},
		},
		{
			description:     "prioritise container field over pod field",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: runtimeDefaultProfile,
		},
		{
			description:   "prioritise container field over pod field",
			podSc:         &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("field-pod-profile.json")}},
			containerSc:   &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("field-cont-profile.json")}},
			containerName: "container1",
			expectedProfile: &runtimeapi.SecurityProfile{
				ProfileType:  runtimeapi.SecurityProfile_Localhost,
				LocalhostRef: seccompLocalhostRef("field-cont-profile.json"),
			},
		},
	}

	for i, test := range tests {
		seccompProfile := m.getSeccompProfile(test.annotation, test.containerName, test.podSc, test.containerSc, false)
		assert.Equal(t, test.expectedProfile, seccompProfile, "TestCase[%d]: %s", i, test.description)
	}
}

func TestGetSeccompProfileDefaultSeccomp(t *testing.T) {
	_, _, m, err := createTestRuntimeManager()
	require.NoError(t, err)

	unconfinedProfile := &runtimeapi.SecurityProfile{
		ProfileType: runtimeapi.SecurityProfile_Unconfined,
	}

	runtimeDefaultProfile := &runtimeapi.SecurityProfile{
		ProfileType: runtimeapi.SecurityProfile_RuntimeDefault,
	}

	tests := []struct {
		description     string
		annotation      map[string]string
		podSc           *v1.PodSecurityContext
		containerSc     *v1.SecurityContext
		containerName   string
		expectedProfile *runtimeapi.SecurityProfile
	}{
		{
			description:     "no seccomp should return RuntimeDefault",
			expectedProfile: runtimeDefaultProfile,
		},
		{
			description:     "pod seccomp profile set to unconfined returns unconfined",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			expectedProfile: unconfinedProfile,
		},
		{
			description:     "container seccomp profile set to unconfined returns unconfined",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			expectedProfile: unconfinedProfile,
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeRuntimeDefault returns runtime/default",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: runtimeDefaultProfile,
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeRuntimeDefault returns runtime/default",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: runtimeDefaultProfile,
		},
		{
			description: "pod seccomp profile set to SeccompProfileTypeLocalhost returns 'localhost/' + LocalhostProfile",
			podSc:       &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("filename")}},
			expectedProfile: &runtimeapi.SecurityProfile{
				ProfileType:  runtimeapi.SecurityProfile_Localhost,
				LocalhostRef: seccompLocalhostRef("filename"),
			},
		},
		{
			description:     "pod seccomp profile set to SeccompProfileTypeLocalhost with empty LocalhostProfile returns unconfined",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost}},
			expectedProfile: unconfinedProfile,
		},
		{
			description:     "container seccomp profile set to SeccompProfileTypeLocalhost with empty LocalhostProfile returns unconfined",
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost}},
			expectedProfile: unconfinedProfile,
		},
		{
			description: "container seccomp profile set to SeccompProfileTypeLocalhost returns 'localhost/' + LocalhostProfile",
			containerSc: &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("filename2")}},
			expectedProfile: &runtimeapi.SecurityProfile{
				ProfileType:  runtimeapi.SecurityProfile_Localhost,
				LocalhostRef: seccompLocalhostRef("filename2"),
			},
		},
		{
			description:     "prioritise container field over pod field",
			podSc:           &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeUnconfined}},
			containerSc:     &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault}},
			expectedProfile: runtimeDefaultProfile,
		},
		{
			description:   "prioritise container field over pod field",
			podSc:         &v1.PodSecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("field-pod-profile.json")}},
			containerSc:   &v1.SecurityContext{SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeLocalhost, LocalhostProfile: getLocal("field-cont-profile.json")}},
			containerName: "container1",
			expectedProfile: &runtimeapi.SecurityProfile{
				ProfileType:  runtimeapi.SecurityProfile_Localhost,
				LocalhostRef: seccompLocalhostRef("field-cont-profile.json"),
			},
		},
	}

	for i, test := range tests {
		seccompProfile := m.getSeccompProfile(test.annotation, test.containerName, test.podSc, test.containerSc, true)
		assert.Equal(t, test.expectedProfile, seccompProfile, "TestCase[%d]: %s", i, test.description)
	}
}

func getLocal(v string) *string {
	return &v
}
