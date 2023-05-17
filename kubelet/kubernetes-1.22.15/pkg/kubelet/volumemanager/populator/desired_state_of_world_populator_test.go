/*
Copyright 2017 The Kubernetes Authors.

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

package populator

import (
	"testing"
	"time"

	"fmt"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetypes "k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	csitrans "k8s.io/csi-translation-lib"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/kubelet/configmap"
	containertest "k8s.io/kubernetes/pkg/kubelet/container/testing"
	kubepod "k8s.io/kubernetes/pkg/kubelet/pod"
	podtest "k8s.io/kubernetes/pkg/kubelet/pod/testing"
	"k8s.io/kubernetes/pkg/kubelet/secret"
	"k8s.io/kubernetes/pkg/kubelet/volumemanager/cache"
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/csimigration"
	volumetesting "k8s.io/kubernetes/pkg/volume/testing"
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/kubernetes/pkg/volume/util/operationexecutor"
	"k8s.io/kubernetes/pkg/volume/util/types"
)

func pluginPVOmittingClient(dswp *desiredStateOfWorldPopulator) {
	fakeClient := &fake.Clientset{}
	fakeClient.AddReactor("get", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return false, nil, nil
	})
	fakeClient.AddReactor("get", "persistentvolumes", func(action core.Action) (bool, runtime.Object, error) {
		return false, nil, nil
	})
	dswp.kubeClient = fakeClient
}

func prepareDswpWithVolume(t *testing.T) (*desiredStateOfWorldPopulator, kubepod.Manager) {
	// create dswp
	mode := v1.PersistentVolumeFilesystem
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
			VolumeMode: &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	dswp, fakePodManager, _, _, _ := createDswpWithVolume(t, pv, pvc)
	return dswp, fakePodManager
}

func TestFindAndAddNewPods_WithRescontructedVolume(t *testing.T) {
	// create dswp
	dswp, fakePodManager := prepareDswpWithVolume(t)

	// create pod
	fakeOuterVolumeName := "dswp-test-volume-name"
	containers := []v1.Container{
		{
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      fakeOuterVolumeName,
					MountPath: "/mnt",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", fakeOuterVolumeName, "file-bound", containers)

	fakePodManager.AddPod(pod)

	podName := util.GetUniquePodName(pod)

	mode := v1.PersistentVolumeFilesystem
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: fakeOuterVolumeName,
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
			VolumeMode: &mode,
		},
	}
	generatedVolumeName := "fake-plugin/" + pod.Spec.Volumes[0].Name
	uniqueVolumeName := v1.UniqueVolumeName(generatedVolumeName)
	expectedOuterVolumeName := "dswp-test-volume-name"

	opts := operationexecutor.MarkVolumeOpts{
		PodName:             podName,
		PodUID:              pod.UID,
		VolumeName:          uniqueVolumeName,
		OuterVolumeSpecName: generatedVolumeName, // fake reconstructed volume
		VolumeGidVolume:     "",
		VolumeSpec:          volume.NewSpecFromPersistentVolume(pv, false),
		VolumeMountState:    operationexecutor.VolumeMounted,
	}
	dswp.actualStateOfWorld.MarkVolumeAsAttached(opts.VolumeName, opts.VolumeSpec, "fake-node", "")
	dswp.actualStateOfWorld.MarkVolumeAsMounted(opts)

	dswp.findAndAddNewPods()

	mountedVolumes := dswp.actualStateOfWorld.GetMountedVolumesForPod(podName)
	found := false
	for _, volume := range mountedVolumes {
		if volume.OuterVolumeSpecName == expectedOuterVolumeName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf(
			"Could not found pod volume %v in the list of actual state of world volumes to mount.",
			expectedOuterVolumeName)
	}

}

func TestFindAndAddNewPods_WithReprocessPodAndVolumeRetrievalError(t *testing.T) {
	// create dswp
	dswp, fakePodManager := prepareDswpWithVolume(t)

	// create pod
	containers := []v1.Container{
		{
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "dswp-test-volume-name",
					MountPath: "/mnt",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "file-bound", containers)

	fakePodManager.AddPod(pod)

	podName := util.GetUniquePodName(pod)

	dswp.findAndAddNewPods()

	if !dswp.podPreviouslyProcessed(podName) {
		t.Fatalf("Failed to record that the volumes for the specified pod: %s have been processed by the populator", podName)
	}
	pluginPVOmittingClient(dswp)

	dswp.ReprocessPod(podName)
	dswp.findAndAddNewPods()

	if !dswp.podPreviouslyProcessed(podName) {
		t.Fatalf("Failed to record that the volumes for the specified pod: %s have been processed by the populator", podName)
	}
	fakePodManager.DeletePod(pod)
}

func TestFindAndAddNewPods_WithVolumeRetrievalError(t *testing.T) {
	// create dswp
	dswp, fakePodManager := prepareDswpWithVolume(t)

	pluginPVOmittingClient(dswp)

	// create pod
	containers := []v1.Container{
		{
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "dswp-test-volume-name",
					MountPath: "/mnt",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "file-bound", containers)

	fakePodManager.AddPod(pod)

	podName := util.GetUniquePodName(pod)

	dswp.findAndAddNewPods()

	if dswp.podPreviouslyProcessed(podName) {
		t.Fatalf("The volumes for the specified pod: %s should not have been processed by the populator", podName)
	}
	if dswp.podHasBeenSeenOnce(podName) {
		t.Fatalf("The volumes for the specified pod: %s should not have been processed by the populator", podName)
	}
}

func TestFindAndAddNewPods_FindAndRemoveDeletedPods(t *testing.T) {
	dswp, fakePodState, pod, expectedVolumeName, _ := prepareDSWPWithPodPV(t)
	podName := util.GetUniquePodName(pod)

	//let the pod be terminated
	podGet, exist := dswp.podManager.GetPodByName(pod.Namespace, pod.Name)
	if !exist {
		t.Fatalf("Failed to get pod by pod name: %s and namespace: %s", pod.Name, pod.Namespace)
	}
	podGet.Status.Phase = v1.PodFailed
	dswp.podManager.DeletePod(pod)

	dswp.findAndRemoveDeletedPods()

	if !dswp.pods.processedPods[podName] {
		t.Fatalf("Pod should not been removed from desired state of world since pod state still thinks it exists")
	}

	fakePodState.removed = map[kubetypes.UID]struct{}{pod.UID: {}}

	// the pod state is marked as removed, so here findAndRemoveDeletedPods() will remove the pod and volumes it is mounted
	dswp.findAndRemoveDeletedPods()
	if dswp.pods.processedPods[podName] {
		t.Fatalf("Failed to remove pods from desired state of world since they no longer exist")
	}

	volumeExists := dswp.desiredStateOfWorld.VolumeExists(expectedVolumeName)
	if volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <false> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := dswp.desiredStateOfWorld.PodExistsInVolume(
		podName, expectedVolumeName); podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <false> Actual: <%v>",
			podExistsInVolume)
	}

	volumesToMount := dswp.desiredStateOfWorld.GetVolumesToMount()
	for _, volume := range volumesToMount {
		if volume.VolumeName == expectedVolumeName {
			t.Fatalf(
				"Found volume %v in the list of desired state of world volumes to mount. Expected not",
				expectedVolumeName)
		}
	}
}

func TestFindAndRemoveDeletedPodsWithActualState(t *testing.T) {
	dswp, fakePodState, pod, expectedVolumeName, _ := prepareDSWPWithPodPV(t)
	fakeASW := dswp.actualStateOfWorld
	podName := util.GetUniquePodName(pod)

	//let the pod be terminated
	podGet, exist := dswp.podManager.GetPodByName(pod.Namespace, pod.Name)
	if !exist {
		t.Fatalf("Failed to get pod by pod name: %s and namespace: %s", pod.Name, pod.Namespace)
	}
	podGet.Status.Phase = v1.PodFailed

	dswp.findAndRemoveDeletedPods()
	// Although Pod status is terminated, pod still exists in pod manager and actual state does not has this pod and volume information
	// desired state populator will fail to delete this pod and volume first
	volumeExists := dswp.desiredStateOfWorld.VolumeExists(expectedVolumeName)
	if !volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <true> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := dswp.desiredStateOfWorld.PodExistsInVolume(
		podName, expectedVolumeName); !podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <true> Actual: <%v>",
			podExistsInVolume)
	}

	// reconcile with actual state so that volume is added into the actual state
	// desired state populator now can successfully delete the pod and volume
	reconcileASW(fakeASW, dswp.desiredStateOfWorld, t)
	dswp.findAndRemoveDeletedPods()
	if !dswp.desiredStateOfWorld.VolumeExists(expectedVolumeName) {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <false> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	fakePodState.removed = map[kubetypes.UID]struct{}{pod.UID: {}}

	// reconcile with actual state so that volume is added into the actual state
	// desired state populator now can successfully delete the pod and volume
	reconcileASW(fakeASW, dswp.desiredStateOfWorld, t)
	dswp.findAndRemoveDeletedPods()
	volumeExists = dswp.desiredStateOfWorld.VolumeExists(expectedVolumeName)
	if volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <false> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := dswp.desiredStateOfWorld.PodExistsInVolume(
		podName, expectedVolumeName); podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <false> Actual: <%v>",
			podExistsInVolume)
	}
}

func TestFindAndRemoveDeletedPodsWithUncertain(t *testing.T) {
	dswp, fakePodState, pod, expectedVolumeName, pv := prepareDSWPWithPodPV(t)
	podName := util.GetUniquePodName(pod)

	//let the pod be terminated
	podGet, exist := dswp.podManager.GetPodByName(pod.Namespace, pod.Name)
	if !exist {
		t.Fatalf("Failed to get pod by pod name: %s and namespace: %s", pod.Name, pod.Namespace)
	}
	podGet.Status.Phase = v1.PodFailed
	dswp.podManager.DeletePod(pod)
	fakePodState.removed = map[kubetypes.UID]struct{}{pod.UID: {}}

	// Add the volume to ASW by reconciling.
	fakeASW := dswp.actualStateOfWorld
	reconcileASW(fakeASW, dswp.desiredStateOfWorld, t)

	// Mark the volume as uncertain
	opts := operationexecutor.MarkVolumeOpts{
		PodName:             util.GetUniquePodName(pod),
		PodUID:              pod.UID,
		VolumeName:          expectedVolumeName,
		OuterVolumeSpecName: "dswp-test-volume-name",
		VolumeGidVolume:     "",
		VolumeSpec:          volume.NewSpecFromPersistentVolume(pv, false),
		VolumeMountState:    operationexecutor.VolumeMountUncertain,
	}
	err := dswp.actualStateOfWorld.MarkVolumeMountAsUncertain(opts)
	if err != nil {
		t.Fatalf("Failed to set the volume as uncertain: %s", err)
	}

	// the pod state now lists the pod as removed, so here findAndRemoveDeletedPods() will remove the pod and volumes it is mounted
	dswp.findAndRemoveDeletedPods()
	if dswp.pods.processedPods[podName] {
		t.Fatalf("Failed to remove pods from desired state of world since they no longer exist")
	}

	volumeExists := dswp.desiredStateOfWorld.VolumeExists(expectedVolumeName)
	if volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <false> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := dswp.desiredStateOfWorld.PodExistsInVolume(
		podName, expectedVolumeName); podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <false> Actual: <%v>",
			podExistsInVolume)
	}

	volumesToMount := dswp.desiredStateOfWorld.GetVolumesToMount()
	for _, volume := range volumesToMount {
		if volume.VolumeName == expectedVolumeName {
			t.Fatalf(
				"Found volume %v in the list of desired state of world volumes to mount. Expected not",
				expectedVolumeName)
		}
	}
}

func prepareDSWPWithPodPV(t *testing.T) (*desiredStateOfWorldPopulator, *fakePodStateProvider, *v1.Pod, v1.UniqueVolumeName, *v1.PersistentVolume) {
	// create dswp
	mode := v1.PersistentVolumeFilesystem
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
			VolumeMode: &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	dswp, fakePodManager, fakesDSW, _, fakePodState := createDswpWithVolume(t, pv, pvc)

	// create pod
	containers := []v1.Container{
		{
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "dswp-test-volume-name",
					MountPath: "/mnt",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "file-bound", containers)

	fakePodManager.AddPod(pod)

	podName := util.GetUniquePodName(pod)

	generatedVolumeName := "fake-plugin/" + pod.Spec.Volumes[0].Name

	dswp.findAndAddNewPods()

	if !dswp.pods.processedPods[podName] {
		t.Fatalf("Failed to record that the volumes for the specified pod: %s have been processed by the populator", podName)
	}

	expectedVolumeName := v1.UniqueVolumeName(generatedVolumeName)

	volumeExists := fakesDSW.VolumeExists(expectedVolumeName)
	if !volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <true> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := fakesDSW.PodExistsInVolume(
		podName, expectedVolumeName); !podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <true> Actual: <%v>",
			podExistsInVolume)
	}

	verifyVolumeExistsInVolumesToMount(
		t, v1.UniqueVolumeName(generatedVolumeName), false /* expectReportedInUse */, fakesDSW)
	return dswp, fakePodState, pod, expectedVolumeName, pv
}

func TestFindAndRemoveNonattachableVolumes(t *testing.T) {
	// create dswp
	mode := v1.PersistentVolumeFilesystem
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
			VolumeMode: &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}

	fakeVolumePluginMgr, fakeVolumePlugin := volumetesting.GetTestKubeletVolumePluginMgr(t)
	dswp, fakePodManager, fakesDSW, _, _ := createDswpWithVolumeWithCustomPluginMgr(t, pv, pvc, fakeVolumePluginMgr)

	// create pod
	containers := []v1.Container{
		{
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "dswp-test-volume-name",
					MountPath: "/mnt",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "file-bound", containers)

	fakePodManager.AddPod(pod)

	podName := util.GetUniquePodName(pod)

	generatedVolumeName := "fake-plugin/" + pod.Spec.Volumes[0].Name

	dswp.findAndAddNewPods()

	if !dswp.pods.processedPods[podName] {
		t.Fatalf("Failed to record that the volumes for the specified pod: %s have been processed by the populator", podName)
	}

	expectedVolumeName := v1.UniqueVolumeName(generatedVolumeName)

	volumeExists := fakesDSW.VolumeExists(expectedVolumeName)
	if !volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <true> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	// change the volume plugin from attachable to non-attachable
	fakeVolumePlugin.NonAttachable = true

	// The volume should still exist
	verifyVolumeExistsInVolumesToMount(
		t, v1.UniqueVolumeName(generatedVolumeName), false /* expectReportedInUse */, fakesDSW)

	dswp.findAndRemoveDeletedPods()
	// After the volume plugin changes to nonattachable, the corresponding volume attachable field should change.
	volumesToMount := fakesDSW.GetVolumesToMount()
	for _, volume := range volumesToMount {
		if volume.VolumeName == expectedVolumeName {
			if volume.PluginIsAttachable {
				t.Fatalf(
					"Volume %v in the list of desired state of world volumes to mount is still attachable. Expected not",
					expectedVolumeName)
			}
		}
	}
}

func TestEphemeralVolumeOwnerCheck(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.GenericEphemeralVolume, true)()

	// create dswp
	pod, pv, pvc := createEphemeralVolumeObjects("dswp-test-pod", "dswp-test-volume-name", false /* not owned */)
	dswp, fakePodManager, _, _, _ := createDswpWithVolume(t, pv, pvc)
	fakePodManager.AddPod(pod)

	podName := util.GetUniquePodName(pod)

	dswp.findAndAddNewPods()
	if dswp.pods.processedPods[podName] {
		t.Fatalf("%s should not have been processed by the populator", podName)
	}
	require.Equal(t,
		[]string{fmt.Sprintf("error processing PVC %s/%s: not the ephemeral PVC for the pod", pvc.Namespace, pvc.Name)},
		dswp.desiredStateOfWorld.PopPodErrors(podName),
	)
}

func TestEphemeralVolumeEnablement(t *testing.T) {
	// create dswp
	pod, pv, pvc := createEphemeralVolumeObjects("dswp-test-pod", "dswp-test-volume-name", true /* owned */)
	dswp, fakePodManager, fakesDSW, _, fakePodState := createDswpWithVolume(t, pv, pvc)
	fakePodManager.AddPod(pod)

	podName := util.GetUniquePodName(pod)
	volumeName := pod.Spec.Volumes[0].Name
	generatedVolumeName := "fake-plugin/" + volumeName

	// Feature disabled -> refuse to process pod.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.GenericEphemeralVolume, false)()
	dswp.findAndAddNewPods()
	if dswp.pods.processedPods[podName] {
		t.Fatalf("%s should not have been processed by the populator", podName)
	}
	require.Equal(t,
		[]string{fmt.Sprintf("volume %s is a generic ephemeral volume, but that feature is disabled in kubelet", volumeName)},
		dswp.desiredStateOfWorld.PopPodErrors(podName),
	)

	// Enabled -> process pod.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.GenericEphemeralVolume, true)()
	dswp.findAndAddNewPods()
	if !dswp.pods.processedPods[podName] {
		t.Fatalf("Failed to record that the volumes for the specified pod: %s have been processed by the populator", podName)
	}

	expectedVolumeName := v1.UniqueVolumeName(generatedVolumeName)

	volumeExists := fakesDSW.VolumeExists(expectedVolumeName)
	if !volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <true> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := fakesDSW.PodExistsInVolume(
		podName, expectedVolumeName); !podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <true> Actual: <%v>",
			podExistsInVolume)
	}

	verifyVolumeExistsInVolumesToMount(
		t, v1.UniqueVolumeName(generatedVolumeName), false /* expectReportedInUse */, fakesDSW)

	//let the pod be terminated
	podGet, exist := fakePodManager.GetPodByName(pod.Namespace, pod.Name)
	if !exist {
		t.Fatalf("Failed to get pod by pod name: %s and namespace: %s", pod.Name, pod.Namespace)
	}
	fakePodState.removed = map[kubetypes.UID]struct{}{podGet.UID: {}}

	// Pretend again that the feature is disabled.
	// Removal of the pod and volumes is expected to work.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.GenericEphemeralVolume, false)()

	dswp.findAndRemoveDeletedPods()
	// Although Pod status is terminated, pod still exists in pod manager and actual state does not has this pod and volume information
	// desired state populator will fail to delete this pod and volume first
	volumeExists = fakesDSW.VolumeExists(expectedVolumeName)
	if !volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <true> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := fakesDSW.PodExistsInVolume(
		podName, expectedVolumeName); !podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <true> Actual: <%v>",
			podExistsInVolume)
	}

	// reconcile with actual state so that volume is added into the actual state
	// desired state populator now can successfully delete the pod and volume
	fakeASW := dswp.actualStateOfWorld
	reconcileASW(fakeASW, fakesDSW, t)
	dswp.findAndRemoveDeletedPods()
	volumeExists = fakesDSW.VolumeExists(expectedVolumeName)
	if volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <false> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := fakesDSW.PodExistsInVolume(
		podName, expectedVolumeName); podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <false> Actual: <%v>",
			podExistsInVolume)
	}
}

func TestFindAndAddNewPods_FindAndRemoveDeletedPods_Valid_Block_VolumeDevices(t *testing.T) {
	// create dswp
	mode := v1.PersistentVolumeBlock
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "block-bound"},
			VolumeMode: &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	dswp, fakePodManager, fakesDSW, _, fakePodState := createDswpWithVolume(t, pv, pvc)

	// create pod
	containers := []v1.Container{
		{
			VolumeDevices: []v1.VolumeDevice{
				{
					Name:       "dswp-test-volume-name",
					DevicePath: "/dev/sdb",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "block-bound", containers)

	fakePodManager.AddPod(pod)

	podName := util.GetUniquePodName(pod)

	generatedVolumeName := "fake-plugin/" + pod.Spec.Volumes[0].Name

	dswp.findAndAddNewPods()

	if !dswp.pods.processedPods[podName] {
		t.Fatalf("Failed to record that the volumes for the specified pod: %s have been processed by the populator", podName)
	}

	expectedVolumeName := v1.UniqueVolumeName(generatedVolumeName)

	volumeExists := fakesDSW.VolumeExists(expectedVolumeName)
	if !volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <true> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := fakesDSW.PodExistsInVolume(
		podName, expectedVolumeName); !podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <true> Actual: <%v>",
			podExistsInVolume)
	}

	verifyVolumeExistsInVolumesToMount(
		t, v1.UniqueVolumeName(generatedVolumeName), false /* expectReportedInUse */, fakesDSW)

	//let the pod be terminated
	podGet, exist := fakePodManager.GetPodByName(pod.Namespace, pod.Name)
	if !exist {
		t.Fatalf("Failed to get pod by pod name: %s and namespace: %s", pod.Name, pod.Namespace)
	}
	podGet.Status.Phase = v1.PodFailed
	fakePodManager.DeletePod(pod)
	fakePodState.removed = map[kubetypes.UID]struct{}{pod.UID: {}}

	//pod is added to fakePodManager but pod state knows the pod is removed, so here findAndRemoveDeletedPods() will remove the pod and volumes it is mounted
	dswp.findAndRemoveDeletedPods()
	if dswp.pods.processedPods[podName] {
		t.Fatalf("Failed to remove pods from desired state of world since they no longer exist")
	}

	volumeExists = fakesDSW.VolumeExists(expectedVolumeName)
	if volumeExists {
		t.Fatalf(
			"VolumeExists(%q) failed. Expected: <false> Actual: <%v>",
			expectedVolumeName,
			volumeExists)
	}

	if podExistsInVolume := fakesDSW.PodExistsInVolume(
		podName, expectedVolumeName); podExistsInVolume {
		t.Fatalf(
			"DSW PodExistsInVolume returned incorrect value. Expected: <false> Actual: <%v>",
			podExistsInVolume)
	}

	volumesToMount := fakesDSW.GetVolumesToMount()
	for _, volume := range volumesToMount {
		if volume.VolumeName == expectedVolumeName {
			t.Fatalf(
				"Found volume %v in the list of desired state of world volumes to mount. Expected not",
				expectedVolumeName)
		}
	}
}

func TestCreateVolumeSpec_Valid_File_VolumeMounts(t *testing.T) {
	// create dswp
	mode := v1.PersistentVolumeFilesystem
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
			VolumeMode: &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
			VolumeMode: &mode,
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	dswp, fakePodManager, _, _, _ := createDswpWithVolume(t, pv, pvc)

	// create pod
	containers := []v1.Container{
		{
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "dswp-test-volume-name",
					MountPath: "/mnt",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "file-bound", containers)

	fakePodManager.AddPod(pod)
	mountsMap, devicesMap := util.GetPodVolumeNames(pod)
	_, volumeSpec, _, err :=
		dswp.createVolumeSpec(pod.Spec.Volumes[0], pod, mountsMap, devicesMap)

	// Assert
	if volumeSpec == nil || err != nil {
		t.Fatalf("Failed to create volumeSpec with combination of filesystem mode and volumeMounts. err: %v", err)
	}
}

func TestCreateVolumeSpec_Valid_Nil_VolumeMounts(t *testing.T) {
	// create dswp
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
			VolumeMode: nil,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
			VolumeMode: nil,
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	dswp, fakePodManager, _, _, _ := createDswpWithVolume(t, pv, pvc)

	// create pod
	containers := []v1.Container{
		{
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "dswp-test-volume-name",
					MountPath: "/mnt",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "file-bound", containers)

	fakePodManager.AddPod(pod)
	mountsMap, devicesMap := util.GetPodVolumeNames(pod)
	_, volumeSpec, _, err :=
		dswp.createVolumeSpec(pod.Spec.Volumes[0], pod, mountsMap, devicesMap)

	// Assert
	if volumeSpec == nil || err != nil {
		t.Fatalf("Failed to create volumeSpec with combination of filesystem mode and volumeMounts. err: %v", err)
	}
}

func TestCreateVolumeSpec_Valid_Block_VolumeDevices(t *testing.T) {
	// create dswp
	mode := v1.PersistentVolumeBlock
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "block-bound"},
			VolumeMode: &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	dswp, fakePodManager, _, _, _ := createDswpWithVolume(t, pv, pvc)

	// create pod
	containers := []v1.Container{
		{
			VolumeDevices: []v1.VolumeDevice{
				{
					Name:       "dswp-test-volume-name",
					DevicePath: "/dev/sdb",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "block-bound", containers)

	fakePodManager.AddPod(pod)
	mountsMap, devicesMap := util.GetPodVolumeNames(pod)
	_, volumeSpec, _, err :=
		dswp.createVolumeSpec(pod.Spec.Volumes[0], pod, mountsMap, devicesMap)

	// Assert
	if volumeSpec == nil || err != nil {
		t.Fatalf("Failed to create volumeSpec with combination of block mode and volumeDevices. err: %v", err)
	}
}

func TestCreateVolumeSpec_Invalid_File_VolumeDevices(t *testing.T) {
	// create dswp
	mode := v1.PersistentVolumeFilesystem
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
			VolumeMode: &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	dswp, fakePodManager, _, _, _ := createDswpWithVolume(t, pv, pvc)

	// create pod
	containers := []v1.Container{
		{
			VolumeDevices: []v1.VolumeDevice{
				{
					Name:       "dswp-test-volume-name",
					DevicePath: "/dev/sdb",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "file-bound", containers)

	fakePodManager.AddPod(pod)
	mountsMap, devicesMap := util.GetPodVolumeNames(pod)
	_, volumeSpec, _, err :=
		dswp.createVolumeSpec(pod.Spec.Volumes[0], pod, mountsMap, devicesMap)

	// Assert
	if volumeSpec != nil || err == nil {
		t.Fatalf("Unexpected volumeMode and volumeMounts/volumeDevices combination is accepted")
	}
}

func TestCreateVolumeSpec_Invalid_Block_VolumeMounts(t *testing.T) {
	// create dswp
	mode := v1.PersistentVolumeBlock
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dswp-test-volume-name",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "block-bound"},
			VolumeMode: &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	dswp, fakePodManager, _, _, _ := createDswpWithVolume(t, pv, pvc)

	// create pod
	containers := []v1.Container{
		{
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "dswp-test-volume-name",
					MountPath: "/mnt",
				},
			},
		},
	}
	pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "block-bound", containers)

	fakePodManager.AddPod(pod)
	mountsMap, devicesMap := util.GetPodVolumeNames(pod)
	_, volumeSpec, _, err :=
		dswp.createVolumeSpec(pod.Spec.Volumes[0], pod, mountsMap, devicesMap)

	// Assert
	if volumeSpec != nil || err == nil {
		t.Fatalf("Unexpected volumeMode and volumeMounts/volumeDevices combination is accepted")
	}
}

func TestCheckVolumeFSResize(t *testing.T) {
	setCapacity := func(pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim, capacity int) {
		pv.Spec.Capacity = volumeCapacity(capacity)
		pvc.Spec.Resources.Requests = volumeCapacity(capacity)
	}

	testcases := []struct {
		resize       func(*testing.T, *v1.PersistentVolume, *v1.PersistentVolumeClaim, *desiredStateOfWorldPopulator)
		verify       func(*testing.T, []v1.UniqueVolumeName, v1.UniqueVolumeName)
		enableResize bool
		readOnlyVol  bool
		volumeMode   v1.PersistentVolumeMode
	}{
		{
			// No resize request for volume, volumes in ASW shouldn't be marked as fsResizeRequired
			resize: func(*testing.T, *v1.PersistentVolume, *v1.PersistentVolumeClaim, *desiredStateOfWorldPopulator) {
			},
			verify: func(t *testing.T, vols []v1.UniqueVolumeName, _ v1.UniqueVolumeName) {
				if len(vols) > 0 {
					t.Errorf("No resize request for any volumes, but found resize required volumes in ASW: %v", vols)
				}
			},
			enableResize: true,
			volumeMode:   v1.PersistentVolumeFilesystem,
		},
		{
			// Disable the feature gate, so volume shouldn't be marked as fsResizeRequired
			resize: func(_ *testing.T, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim, _ *desiredStateOfWorldPopulator) {
				setCapacity(pv, pvc, 2)
			},
			verify: func(t *testing.T, vols []v1.UniqueVolumeName, _ v1.UniqueVolumeName) {
				if len(vols) > 0 {
					t.Errorf("Feature gate disabled, but found resize required volumes in ASW: %v", vols)
				}
			},
			enableResize: false,
			volumeMode:   v1.PersistentVolumeFilesystem,
		},
		{
			// Make volume used as ReadOnly, so volume shouldn't be marked as fsResizeRequired
			resize: func(_ *testing.T, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim, _ *desiredStateOfWorldPopulator) {
				setCapacity(pv, pvc, 2)
			},
			verify: func(t *testing.T, vols []v1.UniqueVolumeName, _ v1.UniqueVolumeName) {
				if len(vols) > 0 {
					t.Errorf("volume mounted as ReadOnly, but found resize required volumes in ASW: %v", vols)
				}
			},
			readOnlyVol:  true,
			enableResize: true,
			volumeMode:   v1.PersistentVolumeFilesystem,
		},
		{
			// Clear ASW, so volume shouldn't be marked as fsResizeRequired because they are not mounted
			resize: func(_ *testing.T, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim, dswp *desiredStateOfWorldPopulator) {
				setCapacity(pv, pvc, 2)
				clearASW(dswp.actualStateOfWorld, dswp.desiredStateOfWorld, t)
			},
			verify: func(t *testing.T, vols []v1.UniqueVolumeName, _ v1.UniqueVolumeName) {
				if len(vols) > 0 {
					t.Errorf("volume hasn't been mounted, but found resize required volumes in ASW: %v", vols)
				}
			},
			enableResize: true,
			volumeMode:   v1.PersistentVolumeFilesystem,
		},
		{
			// volume in ASW should be marked as fsResizeRequired
			resize: func(_ *testing.T, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim, _ *desiredStateOfWorldPopulator) {
				setCapacity(pv, pvc, 2)
			},
			verify: func(t *testing.T, vols []v1.UniqueVolumeName, volName v1.UniqueVolumeName) {
				if len(vols) == 0 {
					t.Fatalf("Request resize for volume, but volume in ASW hasn't been marked as fsResizeRequired")
				}
				if len(vols) != 1 {
					t.Errorf("Some unexpected volumes are marked as fsResizeRequired: %v", vols)
				}
				if vols[0] != volName {
					t.Fatalf("Mark wrong volume as fsResizeRequired: %s", vols[0])
				}
			},
			enableResize: true,
			volumeMode:   v1.PersistentVolumeFilesystem,
		},
		{
			// volume in ASW should be marked as fsResizeRequired
			resize: func(_ *testing.T, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim, _ *desiredStateOfWorldPopulator) {
				setCapacity(pv, pvc, 2)
			},
			verify: func(t *testing.T, vols []v1.UniqueVolumeName, volName v1.UniqueVolumeName) {
				if len(vols) == 0 {
					t.Fatalf("Request resize for volume, but volume in ASW hasn't been marked as fsResizeRequired")
				}
				if len(vols) != 1 {
					t.Errorf("Some unexpected volumes are marked as fsResizeRequired: %v", vols)
				}
				if vols[0] != volName {
					t.Fatalf("Mark wrong volume as fsResizeRequired: %s", vols[0])
				}
			},
			enableResize: true,
			volumeMode:   v1.PersistentVolumeBlock,
		},
	}

	for _, tc := range testcases {
		pv := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dswp-test-volume-name",
			},
			Spec: v1.PersistentVolumeSpec{
				PersistentVolumeSource: v1.PersistentVolumeSource{RBD: &v1.RBDPersistentVolumeSource{}},
				Capacity:               volumeCapacity(1),
				ClaimRef:               &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
				VolumeMode:             &tc.volumeMode,
			},
		}
		pvc := &v1.PersistentVolumeClaim{
			Spec: v1.PersistentVolumeClaimSpec{
				VolumeName: pv.Name,
				Resources: v1.ResourceRequirements{
					Requests: pv.Spec.Capacity,
				},
			},
			Status: v1.PersistentVolumeClaimStatus{
				Phase:    v1.ClaimBound,
				Capacity: pv.Spec.Capacity,
			},
		}

		dswp, fakePodManager, fakeDSW, _, _ := createDswpWithVolume(t, pv, pvc)
		fakeASW := dswp.actualStateOfWorld
		containers := []v1.Container{}

		if tc.volumeMode == v1.PersistentVolumeFilesystem {
			containers = append(containers, v1.Container{
				VolumeMounts: []v1.VolumeMount{
					{
						Name:      pv.Name,
						MountPath: "/mnt",
						ReadOnly:  tc.readOnlyVol,
					},
				},
			})
		} else {
			containers = append(containers, v1.Container{
				VolumeDevices: []v1.VolumeDevice{
					{
						Name:       pv.Name,
						DevicePath: "/mnt/foobar",
					},
				},
			})
		}

		pod := createPodWithVolume("dswp-test-pod", "dswp-test-volume-name", "file-bound", containers)
		pod.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ReadOnly = tc.readOnlyVol
		uniquePodName := types.UniquePodName(pod.UID)
		uniqueVolumeName := v1.UniqueVolumeName("fake-plugin/" + pod.Spec.Volumes[0].Name)

		fakePodManager.AddPod(pod)
		// Fill the dsw to contains volumes and pods.
		dswp.findAndAddNewPods()
		reconcileASW(fakeASW, fakeDSW, t)

		func() {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ExpandInUsePersistentVolumes, tc.enableResize)()

			tc.resize(t, pv, pvc, dswp)

			resizeRequiredVolumes := reprocess(dswp, uniquePodName, fakeDSW, fakeASW)

			tc.verify(t, resizeRequiredVolumes, uniqueVolumeName)
		}()
	}
}

func volumeCapacity(size int) v1.ResourceList {
	return v1.ResourceList{v1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", size))}
}

func reconcileASW(asw cache.ActualStateOfWorld, dsw cache.DesiredStateOfWorld, t *testing.T) {
	for _, volumeToMount := range dsw.GetVolumesToMount() {
		err := asw.MarkVolumeAsAttached(volumeToMount.VolumeName, volumeToMount.VolumeSpec, "", "")
		if err != nil {
			t.Fatalf("Unexpected error when MarkVolumeAsAttached: %v", err)
		}
		markVolumeOpts := operationexecutor.MarkVolumeOpts{
			PodName:             volumeToMount.PodName,
			PodUID:              volumeToMount.Pod.UID,
			VolumeName:          volumeToMount.VolumeName,
			OuterVolumeSpecName: volumeToMount.OuterVolumeSpecName,
			VolumeGidVolume:     volumeToMount.VolumeGidValue,
			VolumeSpec:          volumeToMount.VolumeSpec,
			VolumeMountState:    operationexecutor.VolumeMounted,
		}
		err = asw.MarkVolumeAsMounted(markVolumeOpts)
		if err != nil {
			t.Fatalf("Unexpected error when MarkVolumeAsMounted: %v", err)
		}
	}
}

func clearASW(asw cache.ActualStateOfWorld, dsw cache.DesiredStateOfWorld, t *testing.T) {
	for _, volumeToMount := range dsw.GetVolumesToMount() {
		err := asw.MarkVolumeAsUnmounted(volumeToMount.PodName, volumeToMount.VolumeName)
		if err != nil {
			t.Fatalf("Unexpected error when MarkVolumeAsUnmounted: %v", err)
		}
	}
	for _, volumeToMount := range dsw.GetVolumesToMount() {
		asw.MarkVolumeAsDetached(volumeToMount.VolumeName, "")
	}
}

func reprocess(dswp *desiredStateOfWorldPopulator, uniquePodName types.UniquePodName,
	dsw cache.DesiredStateOfWorld, asw cache.ActualStateOfWorld) []v1.UniqueVolumeName {
	dswp.ReprocessPod(uniquePodName)
	dswp.findAndAddNewPods()
	return getResizeRequiredVolumes(dsw, asw)
}

func getResizeRequiredVolumes(dsw cache.DesiredStateOfWorld, asw cache.ActualStateOfWorld) []v1.UniqueVolumeName {
	resizeRequiredVolumes := []v1.UniqueVolumeName{}
	for _, volumeToMount := range dsw.GetVolumesToMount() {
		_, _, err := asw.PodExistsInVolume(volumeToMount.PodName, volumeToMount.VolumeName)
		if cache.IsFSResizeRequiredError(err) {
			resizeRequiredVolumes = append(resizeRequiredVolumes, volumeToMount.VolumeName)
		}
	}
	return resizeRequiredVolumes
}

func verifyVolumeExistsInVolumesToMount(t *testing.T, expectedVolumeName v1.UniqueVolumeName, expectReportedInUse bool, dsw cache.DesiredStateOfWorld) {
	volumesToMount := dsw.GetVolumesToMount()
	for _, volume := range volumesToMount {
		if volume.VolumeName == expectedVolumeName {
			if volume.ReportedInUse != expectReportedInUse {
				t.Fatalf(
					"Found volume %v in the list of VolumesToMount, but ReportedInUse incorrect. Expected: <%v> Actual: <%v>",
					expectedVolumeName,
					expectReportedInUse,
					volume.ReportedInUse)
			}

			return
		}
	}

	t.Fatalf(
		"Could not find volume %v in the list of desired state of world volumes to mount %+v",
		expectedVolumeName,
		volumesToMount)
}

func createPodWithVolume(pod, pv, pvc string, containers []v1.Container) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod,
			UID:       "dswp-test-pod-uid",
			Namespace: "dswp-test",
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: pv,
					VolumeSource: v1.VolumeSource{
						GCEPersistentDisk: &v1.GCEPersistentDiskVolumeSource{
							PDName: "dswp-test-fake-device",
						},
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc,
						},
					},
				},
			},
			Containers: containers,
		},
		Status: v1.PodStatus{
			Phase: v1.PodPhase("Running"),
		},
	}
}

func createEphemeralVolumeObjects(podName, volumeName string, owned bool) (pod *v1.Pod, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim) {
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			UID:       "dswp-test-pod-uid",
			Namespace: "dswp-test",
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: volumeName,
					VolumeSource: v1.VolumeSource{
						Ephemeral: &v1.EphemeralVolumeSource{
							VolumeClaimTemplate: &v1.PersistentVolumeClaimTemplate{},
						},
					},
				},
			},
			Containers: []v1.Container{
				{
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      volumeName,
							MountPath: "/mnt",
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			Phase: v1.PodPhase("Running"),
		},
	}
	mode := v1.PersistentVolumeFilesystem
	pv = &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeName,
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Namespace: "ns", Name: "file-bound"},
			VolumeMode: &mode,
		},
	}
	pvc = &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName + "-" + volumeName,
			Namespace: pod.Namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "dswp-test-volume-name",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	if owned {
		controller := true
		pvc.OwnerReferences = []metav1.OwnerReference{
			{
				UID:        pod.UID,
				Name:       podName,
				Controller: &controller,
			},
		}
	}
	return
}

func createDswpWithVolume(t *testing.T, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim) (*desiredStateOfWorldPopulator, kubepod.Manager, cache.DesiredStateOfWorld, *containertest.FakeRuntime, *fakePodStateProvider) {
	fakeVolumePluginMgr, _ := volumetesting.GetTestKubeletVolumePluginMgr(t)
	dswp, fakePodManager, fakesDSW, fakeRuntime, fakeStateProvider := createDswpWithVolumeWithCustomPluginMgr(t, pv, pvc, fakeVolumePluginMgr)
	return dswp, fakePodManager, fakesDSW, fakeRuntime, fakeStateProvider
}

type fakePodStateProvider struct {
	terminating map[kubetypes.UID]struct{}
	removed     map[kubetypes.UID]struct{}
}

func (p *fakePodStateProvider) ShouldPodContainersBeTerminating(uid kubetypes.UID) bool {
	_, ok := p.terminating[uid]
	return ok
}

func (p *fakePodStateProvider) ShouldPodRuntimeBeRemoved(uid kubetypes.UID) bool {
	_, ok := p.removed[uid]
	return ok
}

func createDswpWithVolumeWithCustomPluginMgr(t *testing.T, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim,
	fakeVolumePluginMgr *volume.VolumePluginMgr) (*desiredStateOfWorldPopulator, kubepod.Manager, cache.DesiredStateOfWorld, *containertest.FakeRuntime, *fakePodStateProvider) {
	fakeClient := &fake.Clientset{}
	fakeClient.AddReactor("get", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, pvc, nil
	})
	fakeClient.AddReactor("get", "persistentvolumes", func(action core.Action) (bool, runtime.Object, error) {
		return true, pv, nil
	})

	fakeSecretManager := secret.NewFakeManager()
	fakeConfigMapManager := configmap.NewFakeManager()
	fakePodManager := kubepod.NewBasicPodManager(
		podtest.NewFakeMirrorClient(), fakeSecretManager, fakeConfigMapManager)

	fakesDSW := cache.NewDesiredStateOfWorld(fakeVolumePluginMgr)
	fakeASW := cache.NewActualStateOfWorld("fake", fakeVolumePluginMgr)
	fakeRuntime := &containertest.FakeRuntime{}
	fakeStateProvider := &fakePodStateProvider{}

	csiTranslator := csitrans.New()
	dswp := &desiredStateOfWorldPopulator{
		kubeClient:                fakeClient,
		loopSleepDuration:         100 * time.Millisecond,
		getPodStatusRetryDuration: 2 * time.Second,
		podManager:                fakePodManager,
		podStateProvider:          fakeStateProvider,
		desiredStateOfWorld:       fakesDSW,
		actualStateOfWorld:        fakeASW,
		pods: processedPods{
			processedPods: make(map[types.UniquePodName]bool)},
		kubeContainerRuntime:     fakeRuntime,
		keepTerminatedPodVolumes: false,
		csiMigratedPluginManager: csimigration.NewPluginManager(csiTranslator, utilfeature.DefaultFeatureGate),
		intreeToCSITranslator:    csiTranslator,
		volumePluginMgr:          fakeVolumePluginMgr,
	}
	return dswp, fakePodManager, fakesDSW, fakeRuntime, fakeStateProvider
}
