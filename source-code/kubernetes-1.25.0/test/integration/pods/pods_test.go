/*
Copyright 2015 The Kubernetes Authors.

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

package pods

import (
	"context"
	"fmt"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	kubeapiservertesting "k8s.io/kubernetes/cmd/kube-apiserver/app/testing"
	"k8s.io/kubernetes/test/integration"
	"k8s.io/kubernetes/test/integration/framework"
)

func TestPodUpdateActiveDeadlineSeconds(t *testing.T) {
	// Disable ServiceAccount admission plugin as we don't have serviceaccount controller running.
	server := kubeapiservertesting.StartTestServerOrDie(t, nil, []string{"--disable-admission-plugins=ServiceAccount"}, framework.SharedEtcd())
	defer server.TearDownFn()

	client := clientset.NewForConfigOrDie(server.ClientConfig)

	ns := framework.CreateNamespaceOrDie(client, "pod-activedeadline-update", t)
	defer framework.DeleteNamespaceOrDie(client, ns, t)

	var (
		iZero = int64(0)
		i30   = int64(30)
		i60   = int64(60)
		iNeg  = int64(-1)
	)

	prototypePod := func() *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "xxx",
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "fake-name",
						Image: "fakeimage",
					},
				},
			},
		}
	}

	cases := []struct {
		name     string
		original *int64
		update   *int64
		valid    bool
	}{
		{
			name:     "no change, nil",
			original: nil,
			update:   nil,
			valid:    true,
		},
		{
			name:     "no change, set",
			original: &i30,
			update:   &i30,
			valid:    true,
		},
		{
			name:     "change to positive from nil",
			original: nil,
			update:   &i60,
			valid:    true,
		},
		{
			name:     "change to smaller positive",
			original: &i60,
			update:   &i30,
			valid:    true,
		},
		{
			name:     "change to larger positive",
			original: &i30,
			update:   &i60,
			valid:    false,
		},
		{
			name:     "change to negative from positive",
			original: &i30,
			update:   &iNeg,
			valid:    false,
		},
		{
			name:     "change to negative from nil",
			original: nil,
			update:   &iNeg,
			valid:    false,
		},
		// zero is not allowed, must be a positive integer
		{
			name:     "change to zero from positive",
			original: &i30,
			update:   &iZero,
			valid:    false,
		},
		{
			name:     "change to nil from positive",
			original: &i30,
			update:   nil,
			valid:    false,
		},
	}

	for i, tc := range cases {
		pod := prototypePod()
		pod.Spec.ActiveDeadlineSeconds = tc.original
		pod.ObjectMeta.Name = fmt.Sprintf("activedeadlineseconds-test-%v", i)

		if _, err := client.CoreV1().Pods(ns.Name).Create(context.TODO(), pod, metav1.CreateOptions{}); err != nil {
			t.Errorf("Failed to create pod: %v", err)
		}

		pod.Spec.ActiveDeadlineSeconds = tc.update

		_, err := client.CoreV1().Pods(ns.Name).Update(context.TODO(), pod, metav1.UpdateOptions{})
		if tc.valid && err != nil {
			t.Errorf("%v: failed to update pod: %v", tc.name, err)
		} else if !tc.valid && err == nil {
			t.Errorf("%v: unexpected allowed update to pod", tc.name)
		}

		integration.DeletePodOrErrorf(t, client, ns.Name, pod.Name)
	}
}

func TestPodReadOnlyFilesystem(t *testing.T) {
	// Disable ServiceAccount admission plugin as we don't have serviceaccount controller running.
	server := kubeapiservertesting.StartTestServerOrDie(t, nil, []string{"--disable-admission-plugins=ServiceAccount"}, framework.SharedEtcd())
	defer server.TearDownFn()

	client := clientset.NewForConfigOrDie(server.ClientConfig)

	isReadOnly := true
	ns := framework.CreateNamespaceOrDie(client, "pod-readonly-root", t)
	defer framework.DeleteNamespaceOrDie(client, ns, t)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "xxx",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "fake-name",
					Image: "fakeimage",
					SecurityContext: &v1.SecurityContext{
						ReadOnlyRootFilesystem: &isReadOnly,
					},
				},
			},
		},
	}

	if _, err := client.CoreV1().Pods(ns.Name).Create(context.TODO(), pod, metav1.CreateOptions{}); err != nil {
		t.Errorf("Failed to create pod: %v", err)
	}

	integration.DeletePodOrErrorf(t, client, ns.Name, pod.Name)
}

func TestPodCreateEphemeralContainers(t *testing.T) {
	// Disable ServiceAccount admission plugin as we don't have serviceaccount controller running.
	server := kubeapiservertesting.StartTestServerOrDie(t, nil, []string{"--disable-admission-plugins=ServiceAccount"}, framework.SharedEtcd())
	defer server.TearDownFn()

	client := clientset.NewForConfigOrDie(server.ClientConfig)

	ns := framework.CreateNamespaceOrDie(client, "pod-create-ephemeral-containers", t)
	defer framework.DeleteNamespaceOrDie(client, ns, t)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "xxx",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:                     "fake-name",
					Image:                    "fakeimage",
					ImagePullPolicy:          "Always",
					TerminationMessagePolicy: "File",
				},
			},
			EphemeralContainers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
		},
	}

	if _, err := client.CoreV1().Pods(ns.Name).Create(context.TODO(), pod, metav1.CreateOptions{}); err == nil {
		t.Errorf("Unexpected allowed creation of pod with ephemeral containers")
		integration.DeletePodOrErrorf(t, client, ns.Name, pod.Name)
	} else if !strings.HasSuffix(err.Error(), "spec.ephemeralContainers: Forbidden: cannot be set on create") {
		t.Errorf("Unexpected error when creating pod with ephemeral containers: %v", err)
	}
}

// setUpEphemeralContainers creates a pod that has Ephemeral Containers. This is a two step
// process because Ephemeral Containers are not allowed during pod creation.
func setUpEphemeralContainers(podsClient typedv1.PodInterface, pod *v1.Pod, containers []v1.EphemeralContainer) (*v1.Pod, error) {
	result, err := podsClient.Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %v", err)
	}

	if len(containers) == 0 {
		return result, nil
	}

	pod.Spec.EphemeralContainers = containers
	if _, err := podsClient.Update(context.TODO(), pod, metav1.UpdateOptions{}); err == nil {
		return nil, fmt.Errorf("unexpected allowed direct update of ephemeral containers during set up: %v", err)
	}

	result, err = podsClient.UpdateEphemeralContainers(context.TODO(), pod.Name, pod, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update ephemeral containers for test case set up: %v", err)
	}

	return result, nil
}

func TestPodPatchEphemeralContainers(t *testing.T) {
	// Disable ServiceAccount admission plugin as we don't have serviceaccount controller running.
	server := kubeapiservertesting.StartTestServerOrDie(t, nil, []string{"--disable-admission-plugins=ServiceAccount"}, framework.SharedEtcd())
	defer server.TearDownFn()

	client := clientset.NewForConfigOrDie(server.ClientConfig)

	ns := framework.CreateNamespaceOrDie(client, "pod-patch-ephemeral-containers", t)
	defer framework.DeleteNamespaceOrDie(client, ns, t)

	testPod := func(name string) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:                     "fake-name",
						Image:                    "fakeimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
		}
	}

	cases := []struct {
		name      string
		original  []v1.EphemeralContainer
		patchType types.PatchType
		patchBody []byte
		valid     bool
	}{
		{
			name:      "create single container (strategic)",
			original:  nil,
			patchType: types.StrategicMergePatchType,
			patchBody: []byte(`{
				"spec": {
					"ephemeralContainers": [{
						"name": "debugger1",
						"image": "debugimage",
						"imagePullPolicy": "Always",
						"terminationMessagePolicy": "File"
					}]
				}
			}`),
			valid: true,
		},
		{
			name:      "create single container (merge)",
			original:  nil,
			patchType: types.MergePatchType,
			patchBody: []byte(`{
				"spec": {
					"ephemeralContainers":[{
						"name": "debugger1",
						"image": "debugimage",
						"imagePullPolicy": "Always",
						"terminationMessagePolicy": "File"
					}]
				}
			}`),
			valid: true,
		},
		{
			name:      "create single container (JSON)",
			original:  nil,
			patchType: types.JSONPatchType,
			// Because ephemeralContainers is optional, a JSON patch of an empty ephemeralContainers must add the
			// list rather than simply appending to it.
			patchBody: []byte(`[{
				"op":"add",
				"path":"/spec/ephemeralContainers",
				"value":[{
					"name":"debugger1",
					"image":"debugimage",
					"imagePullPolicy": "Always",
					"terminationMessagePolicy": "File"
				}]
			}]`),
			valid: true,
		},
		{
			name: "add single container (strategic)",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			patchType: types.StrategicMergePatchType,
			patchBody: []byte(`{
				"spec": {
					"ephemeralContainers":[{
						"name": "debugger2",
						"image": "debugimage",
						"imagePullPolicy": "Always",
						"terminationMessagePolicy": "File"
					}]
				}
			}`),
			valid: true,
		},
		{
			name: "add single container (merge)",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			patchType: types.MergePatchType,
			patchBody: []byte(`{
				"spec": {
					"ephemeralContainers":[{
						"name": "debugger1",
						"image": "debugimage",
						"imagePullPolicy": "Always",
						"terminationMessagePolicy": "File"
					},{
						"name": "debugger2",
						"image": "debugimage",
						"imagePullPolicy": "Always",
						"terminationMessagePolicy": "File"
					}]
				} 
			}`),
			valid: true,
		},
		{
			name: "add single container (JSON)",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			patchType: types.JSONPatchType,
			patchBody: []byte(`[{
				"op":"add",
				"path":"/spec/ephemeralContainers/-",
				"value":{
					"name":"debugger2",
					"image":"debugimage",
					"imagePullPolicy": "Always",
					"terminationMessagePolicy": "File"
				}
			}]`),
			valid: true,
		},
		{
			name: "remove all containers (merge)",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			patchType: types.MergePatchType,
			patchBody: []byte(`{"spec": {"ephemeralContainers":[]}}`),
			valid:     false,
		},
		{
			name: "remove the single container (JSON)",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			patchType: types.JSONPatchType,
			patchBody: []byte(`[{"op":"remove","path":"/spec/ephemeralContainers/0"}]`),
			valid:     false, // disallowed by policy rather than patch semantics
		},
		{
			name: "remove all containers (JSON)",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			patchType: types.JSONPatchType,
			patchBody: []byte(`[{"op":"remove","path":"/spec/ephemeralContainers"}]`),
			valid:     false, // disallowed by policy rather than patch semantics
		},
	}

	for i, tc := range cases {
		pod := testPod(fmt.Sprintf("ephemeral-container-test-%v", i))
		if _, err := setUpEphemeralContainers(client.CoreV1().Pods(ns.Name), pod, tc.original); err != nil {
			t.Errorf("%v: %v", tc.name, err)
		}

		if _, err := client.CoreV1().Pods(ns.Name).Patch(context.TODO(), pod.Name, tc.patchType, tc.patchBody, metav1.PatchOptions{}, "ephemeralcontainers"); tc.valid && err != nil {
			t.Errorf("%v: failed to update ephemeral containers: %v", tc.name, err)
		} else if !tc.valid && err == nil {
			t.Errorf("%v: unexpected allowed update to ephemeral containers", tc.name)
		}

		integration.DeletePodOrErrorf(t, client, ns.Name, pod.Name)
	}
}

func TestPodUpdateEphemeralContainers(t *testing.T) {
	// Disable ServiceAccount admission plugin as we don't have serviceaccount controller running.
	server := kubeapiservertesting.StartTestServerOrDie(t, nil, []string{"--disable-admission-plugins=ServiceAccount"}, framework.SharedEtcd())
	defer server.TearDownFn()

	client := clientset.NewForConfigOrDie(server.ClientConfig)

	ns := framework.CreateNamespaceOrDie(client, "pod-update-ephemeral-containers", t)
	defer framework.DeleteNamespaceOrDie(client, ns, t)

	testPod := func(name string) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "fake-name",
						Image: "fakeimage",
					},
				},
			},
		}
	}

	cases := []struct {
		name     string
		original []v1.EphemeralContainer
		update   []v1.EphemeralContainer
		valid    bool
	}{
		{
			name:     "no change, nil",
			original: nil,
			update:   nil,
			valid:    true,
		},
		{
			name: "no change, set",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			update: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			valid: true,
		},
		{
			name:     "add single container",
			original: nil,
			update: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			valid: true,
		},
		{
			name: "remove all containers, nil",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			update: nil,
			valid:  false,
		},
		{
			name: "remove all containers, empty",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			update: []v1.EphemeralContainer{},
			valid:  false,
		},
		{
			name: "increase number of containers",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			update: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger2",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			valid: true,
		},
		{
			name: "decrease number of containers",
			original: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger2",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			update: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger1",
						Image:                    "debugimage",
						ImagePullPolicy:          "Always",
						TerminationMessagePolicy: "File",
					},
				},
			},
			valid: false,
		},
	}

	for i, tc := range cases {
		pod, err := setUpEphemeralContainers(client.CoreV1().Pods(ns.Name), testPod(fmt.Sprintf("ephemeral-container-test-%v", i)), tc.original)
		if err != nil {
			t.Errorf("%v: %v", tc.name, err)
		}

		pod.Spec.EphemeralContainers = tc.update
		if _, err := client.CoreV1().Pods(ns.Name).UpdateEphemeralContainers(context.TODO(), pod.Name, pod, metav1.UpdateOptions{}); tc.valid && err != nil {
			t.Errorf("%v: failed to update ephemeral containers: %v", tc.name, err)
		} else if !tc.valid && err == nil {
			t.Errorf("%v: unexpected allowed update to ephemeral containers", tc.name)
		}

		integration.DeletePodOrErrorf(t, client, ns.Name, pod.Name)
	}
}
