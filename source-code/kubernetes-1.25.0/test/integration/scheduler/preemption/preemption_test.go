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

// This file tests preemption functionality of the scheduler.

package preemption

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/klog/v2"
	configv1 "k8s.io/kube-scheduler/config/v1"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/apis/scheduling"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler"
	configtesting "k8s.io/kubernetes/pkg/scheduler/apis/config/testing"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	"k8s.io/kubernetes/plugin/pkg/admission/priority"
	testutils "k8s.io/kubernetes/test/integration/util"
	"k8s.io/utils/pointer"
)

// imported from testutils
var (
	initPausePod                    = testutils.InitPausePod
	createNode                      = testutils.CreateNode
	createPausePod                  = testutils.CreatePausePod
	runPausePod                     = testutils.RunPausePod
	deletePod                       = testutils.DeletePod
	initTest                        = testutils.InitTestSchedulerWithNS
	initTestDisablePreemption       = testutils.InitTestDisablePreemption
	initDisruptionController        = testutils.InitDisruptionController
	waitCachedPodsStable            = testutils.WaitCachedPodsStable
	podIsGettingEvicted             = testutils.PodIsGettingEvicted
	podUnschedulable                = testutils.PodUnschedulable
	waitForPDBsStable               = testutils.WaitForPDBsStable
	waitForPodToScheduleWithTimeout = testutils.WaitForPodToScheduleWithTimeout
	waitForPodUnschedulable         = testutils.WaitForPodUnschedulable
)

const filterPluginName = "filter-plugin"

var lowPriority, mediumPriority, highPriority = int32(100), int32(200), int32(300)

func waitForNominatedNodeNameWithTimeout(cs clientset.Interface, pod *v1.Pod, timeout time.Duration) error {
	if err := wait.Poll(100*time.Millisecond, timeout, func() (bool, error) {
		pod, err := cs.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if len(pod.Status.NominatedNodeName) > 0 {
			return true, nil
		}
		return false, err
	}); err != nil {
		return fmt.Errorf(".status.nominatedNodeName of Pod %v/%v did not get set: %v", pod.Namespace, pod.Name, err)
	}
	return nil
}

func waitForNominatedNodeName(cs clientset.Interface, pod *v1.Pod) error {
	return waitForNominatedNodeNameWithTimeout(cs, pod, wait.ForeverTestTimeout)
}

const tokenFilterName = "token-filter"

type tokenFilter struct {
	Tokens       int
	Unresolvable bool
}

// Name returns name of the plugin.
func (fp *tokenFilter) Name() string {
	return tokenFilterName
}

func (fp *tokenFilter) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod,
	nodeInfo *framework.NodeInfo) *framework.Status {
	if fp.Tokens > 0 {
		fp.Tokens--
		return nil
	}
	status := framework.Unschedulable
	if fp.Unresolvable {
		status = framework.UnschedulableAndUnresolvable
	}
	return framework.NewStatus(status, fmt.Sprintf("can't fit %v", pod.Name))
}

func (fp *tokenFilter) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
	return nil, nil
}

func (fp *tokenFilter) AddPod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod,
	podInfoToAdd *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	fp.Tokens--
	return nil
}

func (fp *tokenFilter) RemovePod(ctx context.Context, state *framework.CycleState, podToSchedule *v1.Pod,
	podInfoToRemove *framework.PodInfo, nodeInfo *framework.NodeInfo) *framework.Status {
	fp.Tokens++
	return nil
}

func (fp *tokenFilter) PreFilterExtensions() framework.PreFilterExtensions {
	return fp
}

var _ framework.FilterPlugin = &tokenFilter{}

// TestPreemption tests a few preemption scenarios.
func TestPreemption(t *testing.T) {
	// Initialize scheduler with a filter plugin.
	var filter tokenFilter
	registry := make(frameworkruntime.Registry)
	err := registry.Register(filterPluginName, func(_ runtime.Object, fh framework.Handle) (framework.Plugin, error) {
		return &filter, nil
	})
	if err != nil {
		t.Fatalf("Error registering a filter: %v", err)
	}
	cfg := configtesting.V1ToInternalWithDefaults(t, configv1.KubeSchedulerConfiguration{
		Profiles: []configv1.KubeSchedulerProfile{{
			SchedulerName: pointer.StringPtr(v1.DefaultSchedulerName),
			Plugins: &configv1.Plugins{
				Filter: configv1.PluginSet{
					Enabled: []configv1.Plugin{
						{Name: filterPluginName},
					},
				},
				PreFilter: configv1.PluginSet{
					Enabled: []configv1.Plugin{
						{Name: filterPluginName},
					},
				},
			},
		}},
	})

	testCtx := testutils.InitTestSchedulerWithOptions(t,
		testutils.InitTestAPIServer(t, "preemption", nil),
		0,
		scheduler.WithProfiles(cfg.Profiles...),
		scheduler.WithFrameworkOutOfTreeRegistry(registry))
	testutils.SyncInformerFactory(testCtx)
	go testCtx.Scheduler.Run(testCtx.Ctx)

	defer testutils.CleanupTest(t, testCtx)
	cs := testCtx.ClientSet

	defaultPodRes := &v1.ResourceRequirements{Requests: v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(100, resource.DecimalSI)},
	}

	maxTokens := 1000
	tests := []struct {
		name                          string
		existingPods                  []*v1.Pod
		pod                           *v1.Pod
		initTokens                    int
		unresolvable                  bool
		preemptedPodIndexes           map[int]struct{}
		enablePodDisruptionConditions bool
	}{
		{
			name:       "basic pod preemption with PodDisruptionConditions enabled",
			initTokens: maxTokens,
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "victim-pod",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(400, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
					},
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(300, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
			preemptedPodIndexes:           map[int]struct{}{0: {}},
			enablePodDisruptionConditions: true,
		},
		{
			name:       "basic pod preemption",
			initTokens: maxTokens,
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "victim-pod",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(400, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
					},
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(300, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
			preemptedPodIndexes: map[int]struct{}{0: {}},
		},
		{
			name:       "basic pod preemption with filter",
			initTokens: 1,
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "victim-pod",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(200, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
					},
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(200, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
			preemptedPodIndexes: map[int]struct{}{0: {}},
		},
		{
			// same as the previous test, but the filter is unresolvable.
			name:         "basic pod preemption with unresolvable filter",
			initTokens:   1,
			unresolvable: true,
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "victim-pod",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(200, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
					},
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(200, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
			preemptedPodIndexes: map[int]struct{}{},
		},
		{
			name:       "preemption is performed to satisfy anti-affinity",
			initTokens: maxTokens,
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name: "pod-0", Namespace: testCtx.NS.Name,
					Priority:  &mediumPriority,
					Labels:    map[string]string{"pod": "p0"},
					Resources: defaultPodRes,
				}),
				initPausePod(&testutils.PausePodConfig{
					Name: "pod-1", Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Labels:    map[string]string{"pod": "p1"},
					Resources: defaultPodRes,
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "pod",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"preemptor"},
											},
										},
									},
									TopologyKey: "node",
								},
							},
						},
					},
				}),
			},
			// A higher priority pod with anti-affinity.
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Labels:    map[string]string{"pod": "preemptor"},
				Resources: defaultPodRes,
				Affinity: &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "pod",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"p0"},
										},
									},
								},
								TopologyKey: "node",
							},
						},
					},
				},
			}),
			preemptedPodIndexes: map[int]struct{}{0: {}, 1: {}},
		},
		{
			// This is similar to the previous case only pod-1 is high priority.
			name:       "preemption is not performed when anti-affinity is not satisfied",
			initTokens: maxTokens,
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name: "pod-0", Namespace: testCtx.NS.Name,
					Priority:  &mediumPriority,
					Labels:    map[string]string{"pod": "p0"},
					Resources: defaultPodRes,
				}),
				initPausePod(&testutils.PausePodConfig{
					Name: "pod-1", Namespace: testCtx.NS.Name,
					Priority:  &highPriority,
					Labels:    map[string]string{"pod": "p1"},
					Resources: defaultPodRes,
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "pod",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"preemptor"},
											},
										},
									},
									TopologyKey: "node",
								},
							},
						},
					},
				}),
			},
			// A higher priority pod with anti-affinity.
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Labels:    map[string]string{"pod": "preemptor"},
				Resources: defaultPodRes,
				Affinity: &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "pod",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"p0"},
										},
									},
								},
								TopologyKey: "node",
							},
						},
					},
				},
			}),
			preemptedPodIndexes: map[int]struct{}{},
		},
	}

	// Create a node with some resources and a label.
	nodeRes := map[v1.ResourceName]string{
		v1.ResourcePods:   "32",
		v1.ResourceCPU:    "500m",
		v1.ResourceMemory: "500",
	}
	nodeObject := st.MakeNode().Name("node1").Capacity(nodeRes).Label("node", "node1").Obj()
	if _, err := createNode(testCtx.ClientSet, nodeObject); err != nil {
		t.Fatalf("Error creating node: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodDisruptionConditions, test.enablePodDisruptionConditions)()
			filter.Tokens = test.initTokens
			filter.Unresolvable = test.unresolvable
			pods := make([]*v1.Pod, len(test.existingPods))
			// Create and run existingPods.
			for i, p := range test.existingPods {
				pods[i], err = runPausePod(cs, p)
				if err != nil {
					t.Fatalf("Error running pause pod: %v", err)
				}
			}
			// Create the "pod".
			preemptor, err := createPausePod(cs, test.pod)
			if err != nil {
				t.Errorf("Error while creating high priority pod: %v", err)
			}
			// Wait for preemption of pods and make sure the other ones are not preempted.
			for i, p := range pods {
				if _, found := test.preemptedPodIndexes[i]; found {
					if err = wait.Poll(time.Second, wait.ForeverTestTimeout, podIsGettingEvicted(cs, p.Namespace, p.Name)); err != nil {
						t.Errorf("Pod %v/%v is not getting evicted.", p.Namespace, p.Name)
					}
					pod, err := cs.CoreV1().Pods(p.Namespace).Get(testCtx.Ctx, p.Name, metav1.GetOptions{})
					if err != nil {
						t.Errorf("Error %v when getting the updated status for pod %v/%v ", err, p.Namespace, p.Name)
					}
					_, cond := podutil.GetPodCondition(&pod.Status, v1.AlphaNoCompatGuaranteeDisruptionTarget)
					if test.enablePodDisruptionConditions == true && cond == nil {
						t.Errorf("Pod %q does not have the expected condition: %q", klog.KObj(pod), v1.AlphaNoCompatGuaranteeDisruptionTarget)
					} else if test.enablePodDisruptionConditions == false && cond != nil {
						t.Errorf("Pod %q has an unexpected condition: %q", klog.KObj(pod), v1.AlphaNoCompatGuaranteeDisruptionTarget)
					}
				} else {
					if p.DeletionTimestamp != nil {
						t.Errorf("Didn't expect pod %v to get preempted.", p.Name)
					}
				}
			}
			// Also check that the preemptor pod gets the NominatedNodeName field set.
			if len(test.preemptedPodIndexes) > 0 {
				if err := waitForNominatedNodeName(cs, preemptor); err != nil {
					t.Errorf("NominatedNodeName field was not set for pod %v: %v", preemptor.Name, err)
				}
			}

			// Cleanup
			pods = append(pods, preemptor)
			testutils.CleanupPods(cs, t, pods)
		})
	}
}

// TestNonPreemption tests NonPreempt option of PriorityClass of scheduler works as expected.
func TestNonPreemption(t *testing.T) {
	var preemptNever = v1.PreemptNever
	// Initialize scheduler.
	testCtx := initTest(t, "non-preemption")
	defer testutils.CleanupTest(t, testCtx)
	cs := testCtx.ClientSet
	tests := []struct {
		name             string
		PreemptionPolicy *v1.PreemptionPolicy
	}{
		{
			name:             "pod preemption will happen",
			PreemptionPolicy: nil,
		},
		{
			name:             "pod preemption will not happen",
			PreemptionPolicy: &preemptNever,
		},
	}
	victim := initPausePod(&testutils.PausePodConfig{
		Name:      "victim-pod",
		Namespace: testCtx.NS.Name,
		Priority:  &lowPriority,
		Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(400, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
		},
	})

	preemptor := initPausePod(&testutils.PausePodConfig{
		Name:      "preemptor-pod",
		Namespace: testCtx.NS.Name,
		Priority:  &highPriority,
		Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(300, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
		},
	})

	// Create a node with some resources
	nodeRes := map[v1.ResourceName]string{
		v1.ResourcePods:   "32",
		v1.ResourceCPU:    "500m",
		v1.ResourceMemory: "500",
	}
	_, err := createNode(testCtx.ClientSet, st.MakeNode().Name("node1").Capacity(nodeRes).Obj())
	if err != nil {
		t.Fatalf("Error creating nodes: %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer testutils.CleanupPods(cs, t, []*v1.Pod{preemptor, victim})
			preemptor.Spec.PreemptionPolicy = test.PreemptionPolicy
			victimPod, err := createPausePod(cs, victim)
			if err != nil {
				t.Fatalf("Error while creating victim: %v", err)
			}
			if err := waitForPodToScheduleWithTimeout(cs, victimPod, 5*time.Second); err != nil {
				t.Fatalf("victim %v should be become scheduled", victimPod.Name)
			}

			preemptorPod, err := createPausePod(cs, preemptor)
			if err != nil {
				t.Fatalf("Error while creating preemptor: %v", err)
			}

			err = waitForNominatedNodeNameWithTimeout(cs, preemptorPod, 5*time.Second)
			// test.PreemptionPolicy == nil means we expect the preemptor to be nominated.
			expect := test.PreemptionPolicy == nil
			// err == nil indicates the preemptor is indeed nominated.
			got := err == nil
			if got != expect {
				t.Errorf("Expect preemptor to be nominated=%v, but got=%v", expect, got)
			}
		})
	}
}

// TestDisablePreemption tests disable pod preemption of scheduler works as expected.
func TestDisablePreemption(t *testing.T) {
	// Initialize scheduler, and disable preemption.
	testCtx := initTestDisablePreemption(t, "disable-preemption")
	defer testutils.CleanupTest(t, testCtx)
	cs := testCtx.ClientSet

	tests := []struct {
		name         string
		existingPods []*v1.Pod
		pod          *v1.Pod
	}{
		{
			name: "pod preemption will not happen",
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "victim-pod",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(400, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
					},
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(300, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
		},
	}

	// Create a node with some resources
	nodeRes := map[v1.ResourceName]string{
		v1.ResourcePods:   "32",
		v1.ResourceCPU:    "500m",
		v1.ResourceMemory: "500",
	}
	_, err := createNode(testCtx.ClientSet, st.MakeNode().Name("node1").Capacity(nodeRes).Obj())
	if err != nil {
		t.Fatalf("Error creating nodes: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pods := make([]*v1.Pod, len(test.existingPods))
			// Create and run existingPods.
			for i, p := range test.existingPods {
				pods[i], err = runPausePod(cs, p)
				if err != nil {
					t.Fatalf("Test [%v]: Error running pause pod: %v", test.name, err)
				}
			}
			// Create the "pod".
			preemptor, err := createPausePod(cs, test.pod)
			if err != nil {
				t.Errorf("Error while creating high priority pod: %v", err)
			}
			// Ensure preemptor should keep unschedulable.
			if err := waitForPodUnschedulable(cs, preemptor); err != nil {
				t.Errorf("Preemptor %v should not become scheduled", preemptor.Name)
			}

			// Ensure preemptor should not be nominated.
			if err := waitForNominatedNodeNameWithTimeout(cs, preemptor, 5*time.Second); err == nil {
				t.Errorf("Preemptor %v should not be nominated", preemptor.Name)
			}

			// Cleanup
			pods = append(pods, preemptor)
			testutils.CleanupPods(cs, t, pods)
		})
	}
}

// This test verifies that system critical priorities are created automatically and resolved properly.
func TestPodPriorityResolution(t *testing.T) {
	admission := priority.NewPlugin()
	testCtx := testutils.InitTestScheduler(t, testutils.InitTestAPIServer(t, "preemption", admission))
	defer testutils.CleanupTest(t, testCtx)
	cs := testCtx.ClientSet

	// Build clientset and informers for controllers.
	externalClientConfig := restclient.CopyConfig(testCtx.KubeConfig)
	externalClientConfig.QPS = -1
	externalClientset := clientset.NewForConfigOrDie(externalClientConfig)
	externalInformers := informers.NewSharedInformerFactory(externalClientset, time.Second)
	admission.SetExternalKubeClientSet(externalClientset)
	admission.SetExternalKubeInformerFactory(externalInformers)

	// Waiting for all controllers to sync
	testutils.SyncInformerFactory(testCtx)
	externalInformers.Start(testCtx.Ctx.Done())
	externalInformers.WaitForCacheSync(testCtx.Ctx.Done())

	// Run all controllers
	go testCtx.Scheduler.Run(testCtx.Ctx)

	tests := []struct {
		Name             string
		PriorityClass    string
		Pod              *v1.Pod
		ExpectedPriority int32
		ExpectedError    error
	}{
		{
			Name:             "SystemNodeCritical priority class",
			PriorityClass:    scheduling.SystemNodeCritical,
			ExpectedPriority: scheduling.SystemCriticalPriority + 1000,
			Pod: initPausePod(&testutils.PausePodConfig{
				Name:              fmt.Sprintf("pod1-%v", scheduling.SystemNodeCritical),
				Namespace:         metav1.NamespaceSystem,
				PriorityClassName: scheduling.SystemNodeCritical,
			}),
		},
		{
			Name:             "SystemClusterCritical priority class",
			PriorityClass:    scheduling.SystemClusterCritical,
			ExpectedPriority: scheduling.SystemCriticalPriority,
			Pod: initPausePod(&testutils.PausePodConfig{
				Name:              fmt.Sprintf("pod2-%v", scheduling.SystemClusterCritical),
				Namespace:         metav1.NamespaceSystem,
				PriorityClassName: scheduling.SystemClusterCritical,
			}),
		},
		{
			Name:             "Invalid priority class should result in error",
			PriorityClass:    "foo",
			ExpectedPriority: scheduling.SystemCriticalPriority,
			Pod: initPausePod(&testutils.PausePodConfig{
				Name:              fmt.Sprintf("pod3-%v", scheduling.SystemClusterCritical),
				Namespace:         metav1.NamespaceSystem,
				PriorityClassName: "foo",
			}),
			ExpectedError: fmt.Errorf("failed to create pause pod: pods \"pod3-system-cluster-critical\" is forbidden: no PriorityClass with name foo was found"),
		},
	}

	// Create a node with some resources
	nodeRes := map[v1.ResourceName]string{
		v1.ResourcePods:   "32",
		v1.ResourceCPU:    "500m",
		v1.ResourceMemory: "500",
	}
	_, err := createNode(testCtx.ClientSet, st.MakeNode().Name("node1").Capacity(nodeRes).Obj())
	if err != nil {
		t.Fatalf("Error creating nodes: %v", err)
	}

	pods := make([]*v1.Pod, 0, len(tests))
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			t.Run(test.Name, func(t *testing.T) {
				pod, err := runPausePod(cs, test.Pod)
				if err != nil {
					if test.ExpectedError == nil {
						t.Fatalf("Test [PodPriority/%v]: Error running pause pod: %v", test.PriorityClass, err)
					}
					if err.Error() != test.ExpectedError.Error() {
						t.Fatalf("Test [PodPriority/%v]: Expected error %v but got error %v", test.PriorityClass, test.ExpectedError, err)
					}
					return
				}
				pods = append(pods, pod)
				if pod.Spec.Priority != nil {
					if *pod.Spec.Priority != test.ExpectedPriority {
						t.Errorf("Expected pod %v to have priority %v but was %v", pod.Name, test.ExpectedPriority, pod.Spec.Priority)
					}
				} else {
					t.Errorf("Expected pod %v to have priority %v but was nil", pod.Name, test.PriorityClass)
				}
			})
		})
	}
	testutils.CleanupPods(cs, t, pods)
	testutils.CleanupNodes(cs, t)
}

func mkPriorityPodWithGrace(tc *testutils.TestContext, name string, priority int32, grace int64) *v1.Pod {
	defaultPodRes := &v1.ResourceRequirements{Requests: v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(100, resource.DecimalSI)},
	}
	pod := initPausePod(&testutils.PausePodConfig{
		Name:      name,
		Namespace: tc.NS.Name,
		Priority:  &priority,
		Labels:    map[string]string{"pod": name},
		Resources: defaultPodRes,
	})
	pod.Spec.TerminationGracePeriodSeconds = &grace
	return pod
}

// This test ensures that while the preempting pod is waiting for the victims to
// terminate, other pending lower priority pods are not scheduled in the room created
// after preemption and while the higher priority pods is not scheduled yet.
func TestPreemptionStarvation(t *testing.T) {
	// Initialize scheduler.
	testCtx := initTest(t, "preemption")
	defer testutils.CleanupTest(t, testCtx)
	cs := testCtx.ClientSet

	tests := []struct {
		name               string
		numExistingPod     int
		numExpectedPending int
		preemptor          *v1.Pod
	}{
		{
			// This test ensures that while the preempting pod is waiting for the victims
			// terminate, other lower priority pods are not scheduled in the room created
			// after preemption and while the higher priority pods is not scheduled yet.
			name:               "starvation test: higher priority pod is scheduled before the lower priority ones",
			numExistingPod:     10,
			numExpectedPending: 5,
			preemptor: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(300, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
		},
	}

	// Create a node with some resources
	nodeRes := map[v1.ResourceName]string{
		v1.ResourcePods:   "32",
		v1.ResourceCPU:    "500m",
		v1.ResourceMemory: "500",
	}
	_, err := createNode(testCtx.ClientSet, st.MakeNode().Name("node1").Capacity(nodeRes).Obj())
	if err != nil {
		t.Fatalf("Error creating nodes: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pendingPods := make([]*v1.Pod, test.numExpectedPending)
			numRunningPods := test.numExistingPod - test.numExpectedPending
			runningPods := make([]*v1.Pod, numRunningPods)
			// Create and run existingPods.
			for i := 0; i < numRunningPods; i++ {
				runningPods[i], err = createPausePod(cs, mkPriorityPodWithGrace(testCtx, fmt.Sprintf("rpod-%v", i), mediumPriority, 0))
				if err != nil {
					t.Fatalf("Error creating pause pod: %v", err)
				}
			}
			// make sure that runningPods are all scheduled.
			for _, p := range runningPods {
				if err := testutils.WaitForPodToSchedule(cs, p); err != nil {
					t.Fatalf("Pod %v/%v didn't get scheduled: %v", p.Namespace, p.Name, err)
				}
			}
			// Create pending pods.
			for i := 0; i < test.numExpectedPending; i++ {
				pendingPods[i], err = createPausePod(cs, mkPriorityPodWithGrace(testCtx, fmt.Sprintf("ppod-%v", i), mediumPriority, 0))
				if err != nil {
					t.Fatalf("Error creating pending pod: %v", err)
				}
			}
			// Make sure that all pending pods are being marked unschedulable.
			for _, p := range pendingPods {
				if err := wait.Poll(100*time.Millisecond, wait.ForeverTestTimeout,
					podUnschedulable(cs, p.Namespace, p.Name)); err != nil {
					t.Errorf("Pod %v/%v didn't get marked unschedulable: %v", p.Namespace, p.Name, err)
				}
			}
			// Create the preemptor.
			preemptor, err := createPausePod(cs, test.preemptor)
			if err != nil {
				t.Errorf("Error while creating the preempting pod: %v", err)
			}
			// Check if .status.nominatedNodeName of the preemptor pod gets set.
			if err := waitForNominatedNodeName(cs, preemptor); err != nil {
				t.Errorf(".status.nominatedNodeName was not set for pod %v/%v: %v", preemptor.Namespace, preemptor.Name, err)
			}
			// Make sure that preemptor is scheduled after preemptions.
			if err := testutils.WaitForPodToScheduleWithTimeout(cs, preemptor, 60*time.Second); err != nil {
				t.Errorf("Preemptor pod %v didn't get scheduled: %v", preemptor.Name, err)
			}
			// Cleanup
			klog.Info("Cleaning up all pods...")
			allPods := pendingPods
			allPods = append(allPods, runningPods...)
			allPods = append(allPods, preemptor)
			testutils.CleanupPods(cs, t, allPods)
		})
	}
}

// TestPreemptionRaces tests that other scheduling events and operations do not
// race with the preemption process.
func TestPreemptionRaces(t *testing.T) {
	// Initialize scheduler.
	testCtx := initTest(t, "preemption-race")
	defer testutils.CleanupTest(t, testCtx)
	cs := testCtx.ClientSet

	tests := []struct {
		name              string
		numInitialPods    int // Pods created and executed before running preemptor
		numAdditionalPods int // Pods created after creating the preemptor
		numRepetitions    int // Repeat the tests to check races
		preemptor         *v1.Pod
	}{
		{
			// This test ensures that while the preempting pod is waiting for the victims
			// terminate, other lower priority pods are not scheduled in the room created
			// after preemption and while the higher priority pods is not scheduled yet.
			name:              "ensures that other pods are not scheduled while preemptor is being marked as nominated (issue #72124)",
			numInitialPods:    2,
			numAdditionalPods: 20,
			numRepetitions:    5,
			preemptor: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(4900, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(4900, resource.DecimalSI)},
				},
			}),
		},
	}

	// Create a node with some resources
	nodeRes := map[v1.ResourceName]string{
		v1.ResourcePods:   "100",
		v1.ResourceCPU:    "5000m",
		v1.ResourceMemory: "5000",
	}
	_, err := createNode(testCtx.ClientSet, st.MakeNode().Name("node1").Capacity(nodeRes).Obj())
	if err != nil {
		t.Fatalf("Error creating nodes: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.numRepetitions <= 0 {
				test.numRepetitions = 1
			}
			for n := 0; n < test.numRepetitions; n++ {
				initialPods := make([]*v1.Pod, test.numInitialPods)
				additionalPods := make([]*v1.Pod, test.numAdditionalPods)
				// Create and run existingPods.
				for i := 0; i < test.numInitialPods; i++ {
					initialPods[i], err = createPausePod(cs, mkPriorityPodWithGrace(testCtx, fmt.Sprintf("rpod-%v", i), mediumPriority, 0))
					if err != nil {
						t.Fatalf("Error creating pause pod: %v", err)
					}
				}
				// make sure that initial Pods are all scheduled.
				for _, p := range initialPods {
					if err := testutils.WaitForPodToSchedule(cs, p); err != nil {
						t.Fatalf("Pod %v/%v didn't get scheduled: %v", p.Namespace, p.Name, err)
					}
				}
				// Create the preemptor.
				klog.Info("Creating the preemptor pod...")
				preemptor, err := createPausePod(cs, test.preemptor)
				if err != nil {
					t.Errorf("Error while creating the preempting pod: %v", err)
				}

				klog.Info("Creating additional pods...")
				for i := 0; i < test.numAdditionalPods; i++ {
					additionalPods[i], err = createPausePod(cs, mkPriorityPodWithGrace(testCtx, fmt.Sprintf("ppod-%v", i), mediumPriority, 0))
					if err != nil {
						t.Fatalf("Error creating pending pod: %v", err)
					}
				}
				// Check that the preemptor pod gets nominated node name.
				if err := waitForNominatedNodeName(cs, preemptor); err != nil {
					t.Errorf(".status.nominatedNodeName was not set for pod %v/%v: %v", preemptor.Namespace, preemptor.Name, err)
				}
				// Make sure that preemptor is scheduled after preemptions.
				if err := testutils.WaitForPodToScheduleWithTimeout(cs, preemptor, 60*time.Second); err != nil {
					t.Errorf("Preemptor pod %v didn't get scheduled: %v", preemptor.Name, err)
				}

				klog.Info("Check unschedulable pods still exists and were never scheduled...")
				for _, p := range additionalPods {
					pod, err := cs.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Name, metav1.GetOptions{})
					if err != nil {
						t.Errorf("Error in getting Pod %v/%v info: %v", p.Namespace, p.Name, err)
					}
					if len(pod.Spec.NodeName) > 0 {
						t.Errorf("Pod %v/%v is already scheduled", p.Namespace, p.Name)
					}
					_, cond := podutil.GetPodCondition(&pod.Status, v1.PodScheduled)
					if cond != nil && cond.Status != v1.ConditionFalse {
						t.Errorf("Pod %v/%v is no longer unschedulable: %v", p.Namespace, p.Name, err)
					}
				}
				// Cleanup
				klog.Info("Cleaning up all pods...")
				allPods := additionalPods
				allPods = append(allPods, initialPods...)
				allPods = append(allPods, preemptor)
				testutils.CleanupPods(cs, t, allPods)
			}
		})
	}
}

const (
	alwaysFailPlugin = "alwaysFailPlugin"
	doNotFailMe      = "do-not-fail-me"
)

// A fake plugin implements PreBind extension point.
// It always fails with an Unschedulable status, unless the pod contains a `doNotFailMe` string.
type alwaysFail struct{}

func (af *alwaysFail) Name() string {
	return alwaysFailPlugin
}

func (af *alwaysFail) PreBind(_ context.Context, _ *framework.CycleState, p *v1.Pod, _ string) *framework.Status {
	if strings.Contains(p.Name, doNotFailMe) {
		return nil
	}
	return framework.NewStatus(framework.Unschedulable)
}

func newAlwaysFail(_ runtime.Object, _ framework.Handle) (framework.Plugin, error) {
	return &alwaysFail{}, nil
}

// TestNominatedNodeCleanUp verifies if a pod's nominatedNodeName is set and unset
// properly in different scenarios.
func TestNominatedNodeCleanUp(t *testing.T) {
	tests := []struct {
		name         string
		nodeCapacity map[v1.ResourceName]string
		// A slice of pods to be created in batch.
		podsToCreate [][]*v1.Pod
		// Each postCheck function is run after each batch of pods' creation.
		postChecks []func(cs clientset.Interface, pod *v1.Pod) error
		// Delete the fake node or not. Optional.
		deleteNode bool
		// Pods to be deleted. Optional.
		podNamesToDelete []string

		// Register dummy plugin to simulate particular scheduling failures. Optional.
		customPlugins     *configv1.Plugins
		outOfTreeRegistry frameworkruntime.Registry
	}{
		{
			name:         "mid-priority pod preempts low-priority pod, followed by a high-priority pod with another preemption",
			nodeCapacity: map[v1.ResourceName]string{v1.ResourceCPU: "5"},
			podsToCreate: [][]*v1.Pod{
				{
					st.MakePod().Name("low-1").Priority(lowPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
					st.MakePod().Name("low-2").Priority(lowPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
					st.MakePod().Name("low-3").Priority(lowPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
					st.MakePod().Name("low-4").Priority(lowPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
				},
				{
					st.MakePod().Name("medium").Priority(mediumPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "4"}).Obj(),
				},
				{
					st.MakePod().Name("high").Priority(highPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "3"}).Obj(),
				},
			},
			postChecks: []func(cs clientset.Interface, pod *v1.Pod) error{
				testutils.WaitForPodToSchedule,
				waitForNominatedNodeName,
				waitForNominatedNodeName,
			},
		},
		{
			name:         "mid-priority pod preempts low-priority pod, followed by a high-priority pod without additional preemption",
			nodeCapacity: map[v1.ResourceName]string{v1.ResourceCPU: "2"},
			podsToCreate: [][]*v1.Pod{
				{
					st.MakePod().Name("low").Priority(lowPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
				},
				{
					st.MakePod().Name("medium").Priority(mediumPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "2"}).Obj(),
				},
				{
					st.MakePod().Name("high").Priority(highPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
				},
			},
			postChecks: []func(cs clientset.Interface, pod *v1.Pod) error{
				testutils.WaitForPodToSchedule,
				waitForNominatedNodeName,
				testutils.WaitForPodToSchedule,
			},
			podNamesToDelete: []string{"low"},
		},
		{
			name:         "mid-priority pod preempts low-priority pod, followed by a node deletion",
			nodeCapacity: map[v1.ResourceName]string{v1.ResourceCPU: "1"},
			podsToCreate: [][]*v1.Pod{
				{
					st.MakePod().Name("low").Priority(lowPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
				},
				{
					st.MakePod().Name("medium").Priority(mediumPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
				},
			},
			postChecks: []func(cs clientset.Interface, pod *v1.Pod) error{
				testutils.WaitForPodToSchedule,
				waitForNominatedNodeName,
			},
			// Delete the node to simulate an ErrNoNodesAvailable error.
			deleteNode:       true,
			podNamesToDelete: []string{"low"},
		},
		{
			name:         "mid-priority pod preempts low-priority pod, but failed the scheduling unexpectedly",
			nodeCapacity: map[v1.ResourceName]string{v1.ResourceCPU: "1"},
			podsToCreate: [][]*v1.Pod{
				{
					st.MakePod().Name(fmt.Sprintf("low-%v", doNotFailMe)).Priority(lowPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
				},
				{
					st.MakePod().Name("medium").Priority(mediumPriority).Req(map[v1.ResourceName]string{v1.ResourceCPU: "1"}).Obj(),
				},
			},
			postChecks: []func(cs clientset.Interface, pod *v1.Pod) error{
				testutils.WaitForPodToSchedule,
				waitForNominatedNodeName,
			},
			podNamesToDelete: []string{fmt.Sprintf("low-%v", doNotFailMe)},
			customPlugins: &configv1.Plugins{
				PreBind: configv1.PluginSet{
					Enabled: []configv1.Plugin{
						{Name: alwaysFailPlugin},
					},
				},
			},
			outOfTreeRegistry: frameworkruntime.Registry{alwaysFailPlugin: newAlwaysFail},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configtesting.V1ToInternalWithDefaults(t, configv1.KubeSchedulerConfiguration{
				Profiles: []configv1.KubeSchedulerProfile{{
					SchedulerName: pointer.StringPtr(v1.DefaultSchedulerName),
					Plugins:       tt.customPlugins,
				}},
			})
			testCtx := initTest(
				t,
				"preemption",
				scheduler.WithProfiles(cfg.Profiles...),
				scheduler.WithFrameworkOutOfTreeRegistry(tt.outOfTreeRegistry),
			)
			t.Cleanup(func() {
				testutils.CleanupTest(t, testCtx)
			})

			cs, ns := testCtx.ClientSet, testCtx.NS.Name
			// Create a node with the specified capacity.
			nodeName := "fake-node"
			if _, err := createNode(cs, st.MakeNode().Name(nodeName).Capacity(tt.nodeCapacity).Obj()); err != nil {
				t.Fatalf("Error creating node %v: %v", nodeName, err)
			}

			// Create pods and run post check if necessary.
			for i, pods := range tt.podsToCreate {
				for _, p := range pods {
					p.Namespace = ns
					if _, err := createPausePod(cs, p); err != nil {
						t.Fatalf("Error creating pod %v: %v", p.Name, err)
					}
				}
				// If necessary, run the post check function.
				if len(tt.postChecks) > i && tt.postChecks[i] != nil {
					for _, p := range pods {
						if err := tt.postChecks[i](cs, p); err != nil {
							t.Fatalf("Pod %v didn't pass the postChecks[%v]: %v", p.Name, i, err)
						}
					}
				}
			}

			// Delete the node if necessary.
			if tt.deleteNode {
				if err := cs.CoreV1().Nodes().Delete(context.TODO(), nodeName, *metav1.NewDeleteOptions(0)); err != nil {
					t.Fatalf("Node %v cannot be deleted: %v", nodeName, err)
				}
			}

			// Force deleting the terminating pods if necessary.
			// This is required if we demand to delete terminating Pods physically.
			for _, podName := range tt.podNamesToDelete {
				if err := deletePod(cs, podName, ns); err != nil {
					t.Fatalf("Pod %v cannot be deleted: %v", podName, err)
				}
			}

			// Verify if .status.nominatedNodeName is cleared.
			if err := wait.Poll(100*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
				pod, err := cs.CoreV1().Pods(ns).Get(context.TODO(), "medium", metav1.GetOptions{})
				if err != nil {
					t.Errorf("Error getting the medium pod: %v", err)
				}
				if len(pod.Status.NominatedNodeName) == 0 {
					return true, nil
				}
				return false, err
			}); err != nil {
				t.Errorf(".status.nominatedNodeName of the medium pod was not cleared: %v", err)
			}
		})
	}
}

func mkMinAvailablePDB(name, namespace string, uid types.UID, minAvailable int, matchLabels map[string]string) *policy.PodDisruptionBudget {
	intMinAvailable := intstr.FromInt(minAvailable)
	return &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: policy.PodDisruptionBudgetSpec{
			MinAvailable: &intMinAvailable,
			Selector:     &metav1.LabelSelector{MatchLabels: matchLabels},
		},
	}
}

func addPodConditionReady(pod *v1.Pod) {
	pod.Status = v1.PodStatus{
		Phase: v1.PodRunning,
		Conditions: []v1.PodCondition{
			{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
		},
	}
}

// TestPDBInPreemption tests PodDisruptionBudget support in preemption.
func TestPDBInPreemption(t *testing.T) {
	// Initialize scheduler.
	testCtx := initTest(t, "preemption-pdb")
	defer testutils.CleanupTest(t, testCtx)
	cs := testCtx.ClientSet

	initDisruptionController(t, testCtx)

	defaultPodRes := &v1.ResourceRequirements{Requests: v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(100, resource.DecimalSI)},
	}
	defaultNodeRes := map[v1.ResourceName]string{
		v1.ResourcePods:   "32",
		v1.ResourceCPU:    "500m",
		v1.ResourceMemory: "500",
	}

	tests := []struct {
		name                string
		nodeCnt             int
		pdbs                []*policy.PodDisruptionBudget
		pdbPodNum           []int32
		existingPods        []*v1.Pod
		pod                 *v1.Pod
		preemptedPodIndexes map[int]struct{}
	}{
		{
			name:    "A non-PDB violating pod is preempted despite its higher priority",
			nodeCnt: 1,
			pdbs: []*policy.PodDisruptionBudget{
				mkMinAvailablePDB("pdb-1", testCtx.NS.Name, types.UID("pdb-1-uid"), 2, map[string]string{"foo": "bar"}),
			},
			pdbPodNum: []int32{2},
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod1",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					Labels:    map[string]string{"foo": "bar"},
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod2",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					Labels:    map[string]string{"foo": "bar"},
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "mid-pod3",
					Namespace: testCtx.NS.Name,
					Priority:  &mediumPriority,
					Resources: defaultPodRes,
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(300, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
			preemptedPodIndexes: map[int]struct{}{2: {}},
		},
		{
			name:    "A node without any PDB violating pods is preferred for preemption",
			nodeCnt: 2,
			pdbs: []*policy.PodDisruptionBudget{
				mkMinAvailablePDB("pdb-1", testCtx.NS.Name, types.UID("pdb-1-uid"), 2, map[string]string{"foo": "bar"}),
			},
			pdbPodNum: []int32{1},
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod1",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					NodeName:  "node-1",
					Labels:    map[string]string{"foo": "bar"},
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "mid-pod2",
					Namespace: testCtx.NS.Name,
					Priority:  &mediumPriority,
					NodeName:  "node-2",
					Resources: defaultPodRes,
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(500, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
			preemptedPodIndexes: map[int]struct{}{1: {}},
		},
		{
			name:    "A node with fewer PDB violating pods is preferred for preemption",
			nodeCnt: 3,
			pdbs: []*policy.PodDisruptionBudget{
				mkMinAvailablePDB("pdb-1", testCtx.NS.Name, types.UID("pdb-1-uid"), 2, map[string]string{"foo1": "bar"}),
				mkMinAvailablePDB("pdb-2", testCtx.NS.Name, types.UID("pdb-2-uid"), 2, map[string]string{"foo2": "bar"}),
			},
			pdbPodNum: []int32{1, 5},
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod1",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					NodeName:  "node-1",
					Labels:    map[string]string{"foo1": "bar"},
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "mid-pod1",
					Namespace: testCtx.NS.Name,
					Priority:  &mediumPriority,
					Resources: defaultPodRes,
					NodeName:  "node-1",
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod2",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					NodeName:  "node-2",
					Labels:    map[string]string{"foo2": "bar"},
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "mid-pod2",
					Namespace: testCtx.NS.Name,
					Priority:  &mediumPriority,
					Resources: defaultPodRes,
					NodeName:  "node-2",
					Labels:    map[string]string{"foo2": "bar"},
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod4",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					NodeName:  "node-3",
					Labels:    map[string]string{"foo2": "bar"},
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod5",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					NodeName:  "node-3",
					Labels:    map[string]string{"foo2": "bar"},
				}),
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod6",
					Namespace: testCtx.NS.Name,
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					NodeName:  "node-3",
					Labels:    map[string]string{"foo2": "bar"},
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:      "preemptor-pod",
				Namespace: testCtx.NS.Name,
				Priority:  &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(500, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(400, resource.DecimalSI)},
				},
			}),
			// The third node is chosen because PDB is not violated for node 3 and the victims have lower priority than node-2.
			preemptedPodIndexes: map[int]struct{}{4: {}, 5: {}, 6: {}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i := 1; i <= test.nodeCnt; i++ {
				nodeName := fmt.Sprintf("node-%v", i)
				_, err := createNode(cs, st.MakeNode().Name(nodeName).Capacity(defaultNodeRes).Obj())
				if err != nil {
					t.Fatalf("Error creating node %v: %v", nodeName, err)
				}
			}

			pods := make([]*v1.Pod, len(test.existingPods))
			var err error
			// Create and run existingPods.
			for i, p := range test.existingPods {
				if pods[i], err = runPausePod(cs, p); err != nil {
					t.Fatalf("Test [%v]: Error running pause pod: %v", test.name, err)
				}
				// Add pod condition ready so that PDB is updated.
				addPodConditionReady(p)
				if _, err := testCtx.ClientSet.CoreV1().Pods(testCtx.NS.Name).UpdateStatus(context.TODO(), p, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			// Wait for Pods to be stable in scheduler cache.
			if err := waitCachedPodsStable(testCtx, test.existingPods); err != nil {
				t.Fatalf("Not all pods are stable in the cache: %v", err)
			}

			// Create PDBs.
			for _, pdb := range test.pdbs {
				_, err := testCtx.ClientSet.PolicyV1().PodDisruptionBudgets(testCtx.NS.Name).Create(context.TODO(), pdb, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create PDB: %v", err)
				}
			}
			// Wait for PDBs to become stable.
			if err := waitForPDBsStable(testCtx, test.pdbs, test.pdbPodNum); err != nil {
				t.Fatalf("Not all pdbs are stable in the cache: %v", err)
			}

			// Create the "pod".
			preemptor, err := createPausePod(cs, test.pod)
			if err != nil {
				t.Errorf("Error while creating high priority pod: %v", err)
			}
			// Wait for preemption of pods and make sure the other ones are not preempted.
			for i, p := range pods {
				if _, found := test.preemptedPodIndexes[i]; found {
					if err = wait.Poll(time.Second, wait.ForeverTestTimeout, podIsGettingEvicted(cs, p.Namespace, p.Name)); err != nil {
						t.Errorf("Test [%v]: Pod %v/%v is not getting evicted.", test.name, p.Namespace, p.Name)
					}
				} else {
					if p.DeletionTimestamp != nil {
						t.Errorf("Test [%v]: Didn't expect pod %v/%v to get preempted.", test.name, p.Namespace, p.Name)
					}
				}
			}
			// Also check if .status.nominatedNodeName of the preemptor pod gets set.
			if len(test.preemptedPodIndexes) > 0 {
				if err := waitForNominatedNodeName(cs, preemptor); err != nil {
					t.Errorf("Test [%v]: .status.nominatedNodeName was not set for pod %v/%v: %v", test.name, preemptor.Namespace, preemptor.Name, err)
				}
			}

			// Cleanup
			pods = append(pods, preemptor)
			testutils.CleanupPods(cs, t, pods)
			cs.PolicyV1().PodDisruptionBudgets(testCtx.NS.Name).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{})
			cs.CoreV1().Nodes().DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{})
		})
	}
}

func initTestPreferNominatedNode(t *testing.T, nsPrefix string, opts ...scheduler.Option) *testutils.TestContext {
	testCtx := testutils.InitTestSchedulerWithOptions(t, testutils.InitTestAPIServer(t, nsPrefix, nil), 0, opts...)
	testutils.SyncInformerFactory(testCtx)
	// wraps the NextPod() method to make it appear the preemption has been done already and the nominated node has been set.
	f := testCtx.Scheduler.NextPod
	testCtx.Scheduler.NextPod = func() (podInfo *framework.QueuedPodInfo) {
		podInfo = f()
		// Scheduler.Next() may return nil when scheduler is shutting down.
		if podInfo != nil {
			podInfo.Pod.Status.NominatedNodeName = "node-1"
		}
		return podInfo
	}
	go testCtx.Scheduler.Run(testCtx.Ctx)
	return testCtx
}

// TestPreferNominatedNode test that if the nominated node pass all the filters, then preemptor pod will run on the nominated node,
// otherwise, it will be scheduled to another node in the cluster that ables to pass all the filters.
func TestPreferNominatedNode(t *testing.T) {
	defaultNodeRes := map[v1.ResourceName]string{
		v1.ResourcePods:   "32",
		v1.ResourceCPU:    "500m",
		v1.ResourceMemory: "500",
	}
	defaultPodRes := &v1.ResourceRequirements{Requests: v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(100, resource.DecimalSI)},
	}
	tests := []struct {
		name         string
		nodeNames    []string
		existingPods []*v1.Pod
		pod          *v1.Pod
		runningNode  string
	}{
		{
			name:      "nominated node released all resource, preemptor is scheduled to the nominated node",
			nodeNames: []string{"node-1", "node-2"},
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod1",
					Priority:  &lowPriority,
					NodeName:  "node-2",
					Resources: defaultPodRes,
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:     "preemptor-pod",
				Priority: &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(500, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
			runningNode: "node-1",
		},
		{
			name:      "nominated node cannot pass all the filters, preemptor should find a different node",
			nodeNames: []string{"node-1", "node-2"},
			existingPods: []*v1.Pod{
				initPausePod(&testutils.PausePodConfig{
					Name:      "low-pod",
					Priority:  &lowPriority,
					Resources: defaultPodRes,
					NodeName:  "node-1",
				}),
			},
			pod: initPausePod(&testutils.PausePodConfig{
				Name:     "preemptor-pod1",
				Priority: &highPriority,
				Resources: &v1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(500, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(200, resource.DecimalSI)},
				},
			}),
			runningNode: "node-2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testCtx := initTestPreferNominatedNode(t, "perfer-nominated-node")
			t.Cleanup(func() {
				testutils.CleanupTest(t, testCtx)
			})
			cs := testCtx.ClientSet
			nsName := testCtx.NS.Name
			var err error
			var preemptor *v1.Pod
			for _, nodeName := range test.nodeNames {
				_, err := createNode(cs, st.MakeNode().Name(nodeName).Capacity(defaultNodeRes).Obj())
				if err != nil {
					t.Fatalf("Error creating node %v: %v", nodeName, err)
				}
			}

			pods := make([]*v1.Pod, len(test.existingPods))
			// Create and run existingPods.
			for i, p := range test.existingPods {
				p.Namespace = nsName
				pods[i], err = runPausePod(cs, p)
				if err != nil {
					t.Fatalf("Error running pause pod: %v", err)
				}
			}
			test.pod.Namespace = nsName
			preemptor, err = createPausePod(cs, test.pod)
			if err != nil {
				t.Errorf("Error while creating high priority pod: %v", err)
			}
			err = wait.Poll(100*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
				preemptor, err = cs.CoreV1().Pods(test.pod.Namespace).Get(context.TODO(), test.pod.Name, metav1.GetOptions{})
				if err != nil {
					t.Errorf("Error getting the preemptor pod info: %v", err)
				}
				if len(preemptor.Spec.NodeName) == 0 {
					return false, err
				}
				return true, nil
			})
			if err != nil {
				t.Errorf("Cannot schedule Pod %v/%v, error: %v", test.pod.Namespace, test.pod.Name, err)
			}
			// Make sure the pod has been scheduled to the right node.
			if preemptor.Spec.NodeName != test.runningNode {
				t.Errorf("Expect pod running on %v, got %v.", test.runningNode, preemptor.Spec.NodeName)
			}
		})
	}
}
