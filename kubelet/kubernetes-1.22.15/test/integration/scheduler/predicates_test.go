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

package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	testutils "k8s.io/kubernetes/test/integration/util"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

// This file tests the scheduler predicates functionality.

const pollInterval = 100 * time.Millisecond

// TestInterPodAffinity verifies that scheduler's inter pod affinity and
// anti-affinity predicate functions works correctly.
func TestInterPodAffinity(t *testing.T) {
	testCtx := initTest(t, "")
	defer testutils.CleanupTest(t, testCtx)

	// Add a few nodes with labels
	nodes, err := createAndWaitForNodesInCache(testCtx, "testnode", st.MakeNode().Label("region", "r1").Label("zone", "z11"), 2)
	if err != nil {
		t.Fatal(err)
	}

	cs := testCtx.ClientSet
	podLabel := map[string]string{"service": "securityscan"}
	podLabel2 := map[string]string{"security": "S1"}

	if err := createNamespacesWithLabels(cs, []string{"ns1", "ns2"}, map[string]string{"team": "team1"}); err != nil {
		t.Fatal(err)
	}
	if err := createNamespacesWithLabels(cs, []string{"ns3"}, map[string]string{"team": "team2"}); err != nil {
		t.Fatal(err)
	}
	defaultNS := "ns1"

	tests := []struct {
		name      string
		pod       *v1.Pod
		pods      []*v1.Pod
		fits      bool
		errorType string
	}{
		{
			name: "validates that a pod with an invalid podAffinity is rejected because of the LabelSelectorRequirement is invalid",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "security",
												Operator: metav1.LabelSelectorOpDoesNotExist,
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			fits:      false,
			errorType: "invalidPod",
		},
		{
			name: "validates that Inter-pod-Affinity is respected if not matching",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "security",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			fits: false,
		},
		{
			name: "validates that InterPodAffinity is respected if matching. requiredDuringSchedulingIgnoredDuringExecution in PodAffinity using In operator that matches the existing pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan", "value2"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename2",
					Labels: podLabel,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					NodeName:   nodes[0].Name,
				},
			},
			},
			fits: true,
		},
		{
			name: "validates that InterPodAffinity is respected if matching. requiredDuringSchedulingIgnoredDuringExecution in PodAffinity using not in operator in labelSelector that matches the existing pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpNotIn,
												Values:   []string{"securityscan3", "value3"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				NodeName:   nodes[0].Name},
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename2",
					Labels: podLabel}}},
			fits: true,
		},
		{
			name: "validates that inter-pod-affinity is respected when pods have different Namespaces",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan", "value2"},
											},
										},
									},
									TopologyKey: "region",
									Namespaces:  []string{"diff-namespace"},
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				NodeName:   nodes[0].Name},
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename2",
					Labels: podLabel, Namespace: "ns2"}}},
			fits: false,
		},
		{
			name: "Doesn't satisfy the PodAffinity because of unmatching labelSelector with the existing pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"antivirusscan", "value2"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				NodeName:   nodes[0].Name}, ObjectMeta: metav1.ObjectMeta{
				Name:   "fakename2",
				Labels: podLabel}}},
			fits: false,
		},
		{
			name: "validates that InterPodAffinity is respected if matching with multiple affinities in multiple RequiredDuringSchedulingIgnoredDuringExecution ",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpExists,
											}, {
												Key:      "wrongkey",
												Operator: metav1.LabelSelectorOpDoesNotExist,
											},
										},
									},
									TopologyKey: "region",
								}, {
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan"},
											}, {
												Key:      "service",
												Operator: metav1.LabelSelectorOpNotIn,
												Values:   []string{"WrongValue"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				NodeName:   nodes[0].Name}, ObjectMeta: metav1.ObjectMeta{
				Name:   "fakename2",
				Labels: podLabel}}},
			fits: true,
		},
		{
			name: "The labelSelector requirements(items of matchExpressions) are ANDed, the pod cannot schedule onto the node because one of the matchExpression items doesn't match.",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabel2,
					Name:   "fakename",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpExists,
											}, {
												Key:      "wrongkey",
												Operator: metav1.LabelSelectorOpDoesNotExist,
											},
										},
									},
									TopologyKey: "region",
								}, {
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan2"},
											}, {
												Key:      "service",
												Operator: metav1.LabelSelectorOpNotIn,
												Values:   []string{"WrongValue"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				NodeName:   nodes[0].Name}, ObjectMeta: metav1.ObjectMeta{
				Name:   "fakename2",
				Labels: podLabel}}},
			fits: false,
		},
		{
			name: "validates that InterPod Affinity and AntiAffinity is respected if matching",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan", "value2"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"antivirusscan", "value2"},
											},
										},
									},
									TopologyKey: "node",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				NodeName:   nodes[0].Name}, ObjectMeta: metav1.ObjectMeta{
				Name:   "fakename2",
				Labels: podLabel}}},
			fits: true,
		},
		{
			name: "satisfies the PodAffinity and PodAntiAffinity and PodAntiAffinity symmetry with the existing pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan", "value2"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"antivirusscan", "value2"},
											},
										},
									},
									TopologyKey: "node",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{
				{
					Spec: v1.PodSpec{
						Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
						NodeName:   nodes[0].Name,
						Affinity: &v1.Affinity{
							PodAntiAffinity: &v1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "service",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{"antivirusscan", "value2"},
												},
											},
										},
										TopologyKey: "node",
									},
								},
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "fakename2",
						Labels: podLabel},
				},
			},
			fits: true,
		},
		{
			name: "satisfies the PodAffinity but doesn't satisfies the PodAntiAffinity with the existing pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel2,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan", "value2"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan", "value2"},
											},
										},
									},
									TopologyKey: "zone",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				NodeName:   nodes[0].Name}, ObjectMeta: metav1.ObjectMeta{
				Name:   "fakename2",
				Labels: podLabel}}},
			fits: false,
		},
		{
			name: "satisfies the PodAffinity and PodAntiAffinity but doesn't satisfies PodAntiAffinity symmetry with the existing pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan", "value2"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"antivirusscan", "value2"},
											},
										},
									},
									TopologyKey: "node",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{
				{
					Spec: v1.PodSpec{
						NodeName:   nodes[0].Name,
						Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
						Affinity: &v1.Affinity{
							PodAntiAffinity: &v1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "service",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{"securityscan", "value3"},
												},
											},
										},
										TopologyKey: "zone",
									},
								},
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "fakename2",
						Labels: podLabel},
				},
			},
			fits: false,
		},
		{
			name: "pod matches its own Label in PodAffinity and that matches the existing pod Labels",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpNotIn,
												Values:   []string{"securityscan", "value2"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{{Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				NodeName:   "machine2"}, ObjectMeta: metav1.ObjectMeta{
				Name:   "fakename2",
				Labels: podLabel}}},
			fits: false,
		},
		{
			name: "Verify that PodAntiAffinity of an existing pod is respected when PodAntiAffinity symmetry is not satisfied with the existing pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fakename",
					Labels: podLabel,
				},
				Spec: v1.PodSpec{Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}}},
			},
			pods: []*v1.Pod{
				{
					Spec: v1.PodSpec{NodeName: nodes[0].Name,
						Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
						Affinity: &v1.Affinity{
							PodAntiAffinity: &v1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "service",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{"securityscan", "value2"},
												},
											},
										},
										TopologyKey: "zone",
									},
								},
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "fakename2",
						Labels: podLabel},
				},
			},
			fits: false,
		},
		{
			name: "Verify that PodAntiAffinity from existing pod is respected when pod statisfies PodAntiAffinity symmetry with the existing pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "fake-name",
					Labels: podLabel,
				},
				Spec: v1.PodSpec{Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}}},
			},
			pods: []*v1.Pod{
				{
					Spec: v1.PodSpec{NodeName: nodes[0].Name,
						Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
						Affinity: &v1.Affinity{
							PodAntiAffinity: &v1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "service",
													Operator: metav1.LabelSelectorOpNotIn,
													Values:   []string{"securityscan", "value2"},
												},
											},
										},
										TopologyKey: "zone",
									},
								},
							},
						},
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "fake-name2",
						Labels: podLabel},
				},
			},
			fits: true,
		},
		{
			name: "nodes[0] and nodes[1] have same topologyKey and label value. nodes[0] has an existing pod that matches the inter pod affinity rule. The new pod can not be scheduled onto either of the two nodes.",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "fake-name2"},
				Spec: v1.PodSpec{
					Containers:   []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					NodeSelector: map[string]string{"region": "r1"},
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "foo",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"abc"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					NodeName:   nodes[0].Name}, ObjectMeta: metav1.ObjectMeta{Name: "fakename", Labels: map[string]string{"foo": "abc"}}},
			},
			fits: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, pod := range test.pods {
				if pod.Namespace == "" {
					pod.Namespace = defaultNS
				}
				createdPod, err := cs.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Error while creating pod: %v", err)
				}
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodScheduled(cs, createdPod.Namespace, createdPod.Name))
				if err != nil {
					t.Errorf("Error while creating pod: %v", err)
				}
			}
			if test.pod.Namespace == "" {
				test.pod.Namespace = defaultNS
			}

			testPod, err := cs.CoreV1().Pods(test.pod.Namespace).Create(context.TODO(), test.pod, metav1.CreateOptions{})
			if err != nil {
				if !(test.errorType == "invalidPod" && apierrors.IsInvalid(err)) {
					t.Fatalf("Error while creating pod: %v", err)
				}
			}

			if test.fits {
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodScheduled(cs, testPod.Namespace, testPod.Name))
			} else {
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, podUnschedulable(cs, testPod.Namespace, testPod.Name))
			}
			if err != nil {
				t.Errorf("Error while trying to fit a pod: %v", err)
			}

			err = cs.CoreV1().Pods(test.pod.Namespace).Delete(context.TODO(), test.pod.Name, *metav1.NewDeleteOptions(0))
			if err != nil {
				t.Errorf("Error while deleting pod: %v", err)
			}
			err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodDeleted(cs, testCtx.NS.Name, test.pod.Name))
			if err != nil {
				t.Errorf("Error while waiting for pod to get deleted: %v", err)
			}
			for _, pod := range test.pods {
				err = cs.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, *metav1.NewDeleteOptions(0))
				if err != nil {
					t.Errorf("Error while deleting pod: %v", err)
				}
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodDeleted(cs, pod.Namespace, pod.Name))
				if err != nil {
					t.Errorf("Error while waiting for pod to get deleted: %v", err)
				}
			}
		})
	}
}

// TestInterPodAffinityWithNamespaceSelector verifies that inter pod affinity with NamespaceSelector works as expected.
// TODO(https://github.com/kubernetes/enhancements/issues/2249): merge with TestInterPodAffinity once NamespaceSelector
// graduates to GA.
func TestInterPodAffinityWithNamespaceSelector(t *testing.T) {
	podLabel := map[string]string{"service": "securityscan"}
	tests := []struct {
		name        string
		pod         *v1.Pod
		existingPod *v1.Pod
		fits        bool
		errorType   string
		disabled    bool
	}{
		{
			name: "MatchingNamespaces",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-ns-selector",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan"},
											},
										},
									},
									NamespaceSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "team",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"team1"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			existingPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fakename2",
					Labels:    podLabel,
					Namespace: "ns2",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				},
			},
			fits: true,
		},
		{
			name: "Disabled",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-ns-selector",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan"},
											},
										},
									},
									NamespaceSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "team",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"team1"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			existingPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fakename2",
					Labels:    podLabel,
					Namespace: "ns2",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				},
			},
			fits:     false,
			disabled: true,
		},
		{
			name: "MismatchingNamespaces",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-ns-selector",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "service",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"securityscan"},
											},
										},
									},
									NamespaceSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "team",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"team1"},
											},
										},
									},
									TopologyKey: "region",
								},
							},
						},
					},
				},
			},
			existingPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fakename2",
					Labels:    podLabel,
					Namespace: "ns3",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "container", Image: imageutils.GetPauseImageName()}},
				},
			},
			fits: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.PodAffinityNamespaceSelector, !test.disabled)()
			testCtx := initTest(t, "")
			defer testutils.CleanupTest(t, testCtx)

			// Add a few nodes with labels
			nodes, err := createAndWaitForNodesInCache(testCtx, "testnode", st.MakeNode().Label("region", "r1").Label("zone", "z11"), 2)
			if err != nil {
				t.Fatal(err)
			}
			test.existingPod.Spec.NodeName = nodes[0].Name

			cs := testCtx.ClientSet

			if err := createNamespacesWithLabels(cs, []string{"ns1", "ns2"}, map[string]string{"team": "team1"}); err != nil {
				t.Fatal(err)
			}
			if err := createNamespacesWithLabels(cs, []string{"ns3"}, map[string]string{"team": "team2"}); err != nil {
				t.Fatal(err)
			}
			defaultNS := "ns1"

			createdPod, err := cs.CoreV1().Pods(test.existingPod.Namespace).Create(context.TODO(), test.existingPod, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Error while creating pod: %v", err)
			}
			err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodScheduled(cs, createdPod.Namespace, createdPod.Name))
			if err != nil {
				t.Errorf("Error while creating pod: %v", err)
			}

			if test.pod.Namespace == "" {
				test.pod.Namespace = defaultNS
			}

			testPod, err := cs.CoreV1().Pods(test.pod.Namespace).Create(context.TODO(), test.pod, metav1.CreateOptions{})
			if err != nil {
				if !(test.errorType == "invalidPod" && apierrors.IsInvalid(err)) {
					t.Fatalf("Error while creating pod: %v", err)
				}
			}

			if test.fits {
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodScheduled(cs, testPod.Namespace, testPod.Name))
			} else {
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, podUnschedulable(cs, testPod.Namespace, testPod.Name))
			}
			if err != nil {
				t.Errorf("Error while trying to fit a pod: %v", err)
			}

			err = cs.CoreV1().Pods(test.pod.Namespace).Delete(context.TODO(), test.pod.Name, *metav1.NewDeleteOptions(0))
			if err != nil {
				t.Errorf("Error while deleting pod: %v", err)
			}
			err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodDeleted(cs, testCtx.NS.Name, test.pod.Name))
			if err != nil {
				t.Errorf("Error while waiting for pod to get deleted: %v", err)
			}
			err = cs.CoreV1().Pods(test.existingPod.Namespace).Delete(context.TODO(), test.existingPod.Name, *metav1.NewDeleteOptions(0))
			if err != nil {
				t.Errorf("Error while deleting pod: %v", err)
			}
			err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodDeleted(cs, test.existingPod.Namespace, test.existingPod.Name))
			if err != nil {
				t.Errorf("Error while waiting for pod to get deleted: %v", err)
			}
		})
	}
}

// TestEvenPodsSpreadPredicate verifies that EvenPodsSpread predicate functions well.
func TestEvenPodsSpreadPredicate(t *testing.T) {
	testCtx := initTest(t, "eps-predicate")
	cs := testCtx.ClientSet
	ns := testCtx.NS.Name
	defer testutils.CleanupTest(t, testCtx)

	for i := 0; i < 4; i++ {
		// Create nodes with labels "zone: zone-{0,1}" and "node: <node name>" to each node.
		nodeName := fmt.Sprintf("node-%d", i)
		zone := fmt.Sprintf("zone-%d", i/2)
		_, err := createNode(cs, st.MakeNode().Name(nodeName).Label("node", nodeName).Label("zone", zone).Obj())
		if err != nil {
			t.Fatalf("Cannot create node: %v", err)
		}
	}

	pause := imageutils.GetPauseImageName()
	tests := []struct {
		name           string
		incomingPod    *v1.Pod
		existingPods   []*v1.Pod
		fits           bool
		candidateNodes []string // nodes expected to schedule onto
	}{
		// note: naming starts at index 0
		{
			name: "place pod on a 1/1/0/1 cluster with MaxSkew=1, node-2 is the only fit",
			incomingPod: st.MakePod().Namespace(ns).Name("p").Label("foo", "").Container(pause).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Namespace(ns).Name("p0").Node("node-0").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p1").Node("node-1").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p3").Node("node-3").Label("foo", "").Container(pause).Obj(),
			},
			fits:           true,
			candidateNodes: []string{"node-2"},
		},
		{
			name: "place pod on a 2/0/0/1 cluster with MaxSkew=2, node-{1,2,3} are good fits",
			incomingPod: st.MakePod().Namespace(ns).Name("p").Label("foo", "").Container(pause).
				SpreadConstraint(2, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Namespace(ns).Name("p0a").Node("node-0").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p0b").Node("node-0").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p3").Node("node-3").Label("foo", "").Container(pause).Obj(),
			},
			fits:           true,
			candidateNodes: []string{"node-1", "node-2", "node-3"},
		},
		{
			name: "pod is required to be placed on zone0, so only node-1 fits",
			incomingPod: st.MakePod().Namespace(ns).Name("p").Label("foo", "").Container(pause).
				NodeAffinityIn("zone", []string{"zone-0"}).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Namespace(ns).Name("p0").Node("node-0").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p3").Node("node-3").Label("foo", "").Container(pause).Obj(),
			},
			fits:           true,
			candidateNodes: []string{"node-1"},
		},
		{
			name: "two constraints: pod can only be placed to zone-1/node-2",
			incomingPod: st.MakePod().Namespace(ns).Name("p").Label("foo", "").Container(pause).
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Namespace(ns).Name("p0").Node("node-0").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p1").Node("node-1").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p3a").Node("node-3").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p3b").Node("node-3").Label("foo", "").Container(pause).Obj(),
			},
			fits:           true,
			candidateNodes: []string{"node-2"},
		},
		{
			name: "pod cannot be placed onto any node",
			incomingPod: st.MakePod().Namespace(ns).Name("p").Label("foo", "").Container(pause).
				NodeAffinityNotIn("node", []string{"node-0"}). // mock a 3-node cluster
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Namespace(ns).Name("p1a").Node("node-1").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p1b").Node("node-1").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p2a").Node("node-2").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p2b").Node("node-2").Label("foo", "").Container(pause).Obj(),
				st.MakePod().Namespace(ns).Name("p3").Node("node-3").Label("foo", "").Container(pause).Obj(),
			},
			fits: false,
		},
		{
			name: "high priority pod can preempt others",
			incomingPod: st.MakePod().Namespace(ns).Name("p").Label("foo", "").Container(pause).Priority(100).
				NodeAffinityNotIn("node", []string{"node-0"}). // mock a 3-node cluster
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().ZeroTerminationGracePeriod().Namespace(ns).Name("p1a").Node("node-1").Label("foo", "").Container(pause).Obj(),
				st.MakePod().ZeroTerminationGracePeriod().Namespace(ns).Name("p1b").Node("node-1").Label("foo", "").Container(pause).Obj(),
				st.MakePod().ZeroTerminationGracePeriod().Namespace(ns).Name("p2a").Node("node-2").Label("foo", "").Container(pause).Obj(),
				st.MakePod().ZeroTerminationGracePeriod().Namespace(ns).Name("p2b").Node("node-2").Label("foo", "").Container(pause).Obj(),
				st.MakePod().ZeroTerminationGracePeriod().Namespace(ns).Name("p3").Node("node-3").Label("foo", "").Container(pause).Obj(),
			},
			fits:           true,
			candidateNodes: []string{"node-1", "node-2", "node-3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allPods := append(tt.existingPods, tt.incomingPod)
			defer testutils.CleanupPods(cs, t, allPods)
			for _, pod := range tt.existingPods {
				createdPod, err := cs.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Error while creating pod during test: %v", err)
				}
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, testutils.PodScheduled(cs, createdPod.Namespace, createdPod.Name))
				if err != nil {
					t.Errorf("Error while waiting for pod during test: %v", err)
				}
			}
			testPod, err := cs.CoreV1().Pods(tt.incomingPod.Namespace).Create(context.TODO(), tt.incomingPod, metav1.CreateOptions{})
			if err != nil && !apierrors.IsInvalid(err) {
				t.Fatalf("Error while creating pod during test: %v", err)
			}

			if tt.fits {
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, podScheduledIn(cs, testPod.Namespace, testPod.Name, tt.candidateNodes))
			} else {
				err = wait.Poll(pollInterval, wait.ForeverTestTimeout, podUnschedulable(cs, testPod.Namespace, testPod.Name))
			}
			if err != nil {
				t.Errorf("Test Failed: %v", err)
			}
		})
	}
}

var (
	hardSpread = v1.DoNotSchedule
	softSpread = v1.ScheduleAnyway
)

func TestUnschedulablePodBecomesSchedulable(t *testing.T) {
	tests := []struct {
		name   string
		init   func(kubernetes.Interface, string) error
		pod    *pausePodConfig
		update func(kubernetes.Interface, string) error
	}{
		{
			name: "node gets added",
			pod: &pausePodConfig{
				Name: "pod-1",
			},
			update: func(cs kubernetes.Interface, _ string) error {
				_, err := createNode(cs, st.MakeNode().Name("node-added").Obj())
				if err != nil {
					return fmt.Errorf("cannot create node: %v", err)
				}
				return nil
			},
		},
		{
			name: "node gets taint removed",
			init: func(cs kubernetes.Interface, _ string) error {
				node, err := createNode(cs, st.MakeNode().Name("node-tainted").Obj())
				if err != nil {
					return fmt.Errorf("cannot create node: %v", err)
				}
				taint := v1.Taint{Key: "test", Value: "test", Effect: v1.TaintEffectNoSchedule}
				if err := testutils.AddTaintToNode(cs, node.Name, taint); err != nil {
					return fmt.Errorf("cannot add taint to node: %v", err)
				}
				return nil
			},
			pod: &pausePodConfig{
				Name: "pod-1",
			},
			update: func(cs kubernetes.Interface, _ string) error {
				taint := v1.Taint{Key: "test", Value: "test", Effect: v1.TaintEffectNoSchedule}
				if err := testutils.RemoveTaintOffNode(cs, "node-tainted", taint); err != nil {
					return fmt.Errorf("cannot remove taint off node: %v", err)
				}
				return nil
			},
		},
		{
			name: "other pod gets deleted",
			init: func(cs kubernetes.Interface, ns string) error {
				nodeObject := st.MakeNode().Name("node-scheduler-integration-test").Capacity(map[v1.ResourceName]string{v1.ResourcePods: "1"}).Obj()
				_, err := createNode(cs, nodeObject)
				if err != nil {
					return fmt.Errorf("cannot create node: %v", err)
				}
				_, err = createPausePod(cs, initPausePod(&pausePodConfig{Name: "pod-to-be-deleted", Namespace: ns}))
				if err != nil {
					return fmt.Errorf("cannot create pod: %v", err)
				}
				return nil
			},
			pod: &pausePodConfig{
				Name: "pod-1",
			},
			update: func(cs kubernetes.Interface, ns string) error {
				if err := deletePod(cs, "pod-to-be-deleted", ns); err != nil {
					return fmt.Errorf("cannot delete pod: %v", err)
				}
				return nil
			},
		},
		{
			name: "pod with pod-affinity gets added",
			init: func(cs kubernetes.Interface, _ string) error {
				_, err := createNode(cs, st.MakeNode().Name("node-1").Label("region", "test").Obj())
				if err != nil {
					return fmt.Errorf("cannot create node: %v", err)
				}
				return nil
			},
			pod: &pausePodConfig{
				Name: "pod-1",
				Affinity: &v1.Affinity{
					PodAffinity: &v1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-with-affinity": "true",
									},
								},
								TopologyKey: "region",
							},
						},
					},
				},
			},
			update: func(cs kubernetes.Interface, ns string) error {
				podConfig := &pausePodConfig{
					Name:      "pod-with-affinity",
					Namespace: ns,
					Labels: map[string]string{
						"pod-with-affinity": "true",
					},
				}
				if _, err := createPausePod(cs, initPausePod(podConfig)); err != nil {
					return fmt.Errorf("cannot create pod: %v", err)
				}
				return nil
			},
		},
		{
			name: "scheduled pod gets updated to match affinity",
			init: func(cs kubernetes.Interface, ns string) error {
				_, err := createNode(cs, st.MakeNode().Name("node-1").Label("region", "test").Obj())
				if err != nil {
					return fmt.Errorf("cannot create node: %v", err)
				}
				if _, err := createPausePod(cs, initPausePod(&pausePodConfig{Name: "pod-to-be-updated", Namespace: ns})); err != nil {
					return fmt.Errorf("cannot create pod: %v", err)
				}
				return nil
			},
			pod: &pausePodConfig{
				Name: "pod-1",
				Affinity: &v1.Affinity{
					PodAffinity: &v1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-with-affinity": "true",
									},
								},
								TopologyKey: "region",
							},
						},
					},
				},
			},
			update: func(cs kubernetes.Interface, ns string) error {
				pod, err := getPod(cs, "pod-to-be-updated", ns)
				if err != nil {
					return fmt.Errorf("cannot get pod: %v", err)
				}
				pod.Labels = map[string]string{"pod-with-affinity": "true"}
				if _, err := cs.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{}); err != nil {
					return fmt.Errorf("cannot update pod: %v", err)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCtx := initTest(t, "scheduler-informer")
			defer testutils.CleanupTest(t, testCtx)

			if tt.init != nil {
				if err := tt.init(testCtx.ClientSet, testCtx.NS.Name); err != nil {
					t.Fatal(err)
				}
			}
			tt.pod.Namespace = testCtx.NS.Name
			pod, err := createPausePod(testCtx.ClientSet, initPausePod(tt.pod))
			if err != nil {
				t.Fatal(err)
			}
			if err := waitForPodUnschedulable(testCtx.ClientSet, pod); err != nil {
				t.Errorf("Pod %v got scheduled: %v", pod.Name, err)
			}
			if err := tt.update(testCtx.ClientSet, testCtx.NS.Name); err != nil {
				t.Fatal(err)
			}
			if err := testutils.WaitForPodToSchedule(testCtx.ClientSet, pod); err != nil {
				t.Errorf("Pod %v was not scheduled: %v", pod.Name, err)
			}
			// Make sure pending queue is empty.
			pendingPods := len(testCtx.Scheduler.SchedulingQueue.PendingPods())
			if pendingPods != 0 {
				t.Errorf("pending pods queue is not empty, size is: %d", pendingPods)
			}
		})
	}
}
