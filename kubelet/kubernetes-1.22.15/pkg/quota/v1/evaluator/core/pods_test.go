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

package core

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/clock"
	quota "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/apiserver/pkg/quota/v1/generic"
	"k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/util/node"
)

func TestPodConstraintsFunc(t *testing.T) {
	testCases := map[string]struct {
		pod      *api.Pod
		required []corev1.ResourceName
		err      string
	}{
		"init container resource missing": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					InitContainers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceCPU: resource.MustParse("1m")},
							Limits:   api.ResourceList{api.ResourceCPU: resource.MustParse("2m")},
						},
					}},
				},
			},
			required: []corev1.ResourceName{corev1.ResourceMemory},
			err:      `must specify memory`,
		},
		"container resource missing": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceCPU: resource.MustParse("1m")},
							Limits:   api.ResourceList{api.ResourceCPU: resource.MustParse("2m")},
						},
					}},
				},
			},
			required: []corev1.ResourceName{corev1.ResourceMemory},
			err:      `must specify memory`,
		},
	}
	evaluator := NewPodEvaluator(nil, clock.RealClock{})
	for testName, test := range testCases {
		err := evaluator.Constraints(test.required, test.pod)
		switch {
		case err != nil && len(test.err) == 0,
			err == nil && len(test.err) != 0,
			err != nil && test.err != err.Error():
			t.Errorf("%s unexpected error: %v", testName, err)
		}
	}
}

func TestPodEvaluatorUsage(t *testing.T) {
	fakeClock := clock.NewFakeClock(time.Now())
	evaluator := NewPodEvaluator(nil, fakeClock)

	// fields use to simulate a pod undergoing termination
	// note: we set the deletion time in the past
	now := fakeClock.Now()
	terminationGracePeriodSeconds := int64(30)
	deletionTimestampPastGracePeriod := metav1.NewTime(now.Add(time.Duration(terminationGracePeriodSeconds) * time.Second * time.Duration(-2)))
	deletionTimestampNotPastGracePeriod := metav1.NewTime(fakeClock.Now())

	testCases := map[string]struct {
		pod                *api.Pod
		usage              corev1.ResourceList
		podOverheadEnabled bool
	}{
		"init container CPU": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					InitContainers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceCPU: resource.MustParse("1m")},
							Limits:   api.ResourceList{api.ResourceCPU: resource.MustParse("2m")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("1m"),
				corev1.ResourceLimitsCPU:   resource.MustParse("2m"),
				corev1.ResourcePods:        resource.MustParse("1"),
				corev1.ResourceCPU:         resource.MustParse("1m"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"init container MEM": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					InitContainers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceMemory: resource.MustParse("1m")},
							Limits:   api.ResourceList{api.ResourceMemory: resource.MustParse("2m")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceRequestsMemory: resource.MustParse("1m"),
				corev1.ResourceLimitsMemory:   resource.MustParse("2m"),
				corev1.ResourcePods:           resource.MustParse("1"),
				corev1.ResourceMemory:         resource.MustParse("1m"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"init container local ephemeral storage": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					InitContainers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceEphemeralStorage: resource.MustParse("32Mi")},
							Limits:   api.ResourceList{api.ResourceEphemeralStorage: resource.MustParse("64Mi")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceEphemeralStorage:         resource.MustParse("32Mi"),
				corev1.ResourceRequestsEphemeralStorage: resource.MustParse("32Mi"),
				corev1.ResourceLimitsEphemeralStorage:   resource.MustParse("64Mi"),
				corev1.ResourcePods:                     resource.MustParse("1"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"init container hugepages": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					InitContainers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceName(api.ResourceHugePagesPrefix + "2Mi"): resource.MustParse("100Mi")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceHugePagesPrefix + "2Mi"):         resource.MustParse("100Mi"),
				corev1.ResourceName(corev1.ResourceRequestsHugePagesPrefix + "2Mi"): resource.MustParse("100Mi"),
				corev1.ResourcePods: resource.MustParse("1"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"init container extended resources": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					InitContainers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceName("example.com/dongle"): resource.MustParse("3")},
							Limits:   api.ResourceList{api.ResourceName("example.com/dongle"): resource.MustParse("3")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceName("requests.example.com/dongle"): resource.MustParse("3"),
				corev1.ResourcePods: resource.MustParse("1"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"container CPU": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceCPU: resource.MustParse("1m")},
							Limits:   api.ResourceList{api.ResourceCPU: resource.MustParse("2m")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("1m"),
				corev1.ResourceLimitsCPU:   resource.MustParse("2m"),
				corev1.ResourcePods:        resource.MustParse("1"),
				corev1.ResourceCPU:         resource.MustParse("1m"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"container MEM": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceMemory: resource.MustParse("1m")},
							Limits:   api.ResourceList{api.ResourceMemory: resource.MustParse("2m")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceRequestsMemory: resource.MustParse("1m"),
				corev1.ResourceLimitsMemory:   resource.MustParse("2m"),
				corev1.ResourcePods:           resource.MustParse("1"),
				corev1.ResourceMemory:         resource.MustParse("1m"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"container local ephemeral storage": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceEphemeralStorage: resource.MustParse("32Mi")},
							Limits:   api.ResourceList{api.ResourceEphemeralStorage: resource.MustParse("64Mi")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceEphemeralStorage:         resource.MustParse("32Mi"),
				corev1.ResourceRequestsEphemeralStorage: resource.MustParse("32Mi"),
				corev1.ResourceLimitsEphemeralStorage:   resource.MustParse("64Mi"),
				corev1.ResourcePods:                     resource.MustParse("1"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"container hugepages": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceName(api.ResourceHugePagesPrefix + "2Mi"): resource.MustParse("100Mi")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceName(api.ResourceHugePagesPrefix + "2Mi"):         resource.MustParse("100Mi"),
				corev1.ResourceName(api.ResourceRequestsHugePagesPrefix + "2Mi"): resource.MustParse("100Mi"),
				corev1.ResourcePods: resource.MustParse("1"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"container extended resources": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceName("example.com/dongle"): resource.MustParse("3")},
							Limits:   api.ResourceList{api.ResourceName("example.com/dongle"): resource.MustParse("3")},
						},
					}},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceName("requests.example.com/dongle"): resource.MustParse("3"),
				corev1.ResourcePods: resource.MustParse("1"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"terminated generic count still appears": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{api.ResourceName("example.com/dongle"): resource.MustParse("3")},
							Limits:   api.ResourceList{api.ResourceName("example.com/dongle"): resource.MustParse("3")},
						},
					}},
				},
				Status: api.PodStatus{
					Phase: api.PodSucceeded,
				},
			},
			usage: corev1.ResourceList{
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"init container maximums override sum of containers": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					InitContainers: []api.Container{
						{
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									api.ResourceCPU:                        resource.MustParse("4"),
									api.ResourceMemory:                     resource.MustParse("100M"),
									api.ResourceName("example.com/dongle"): resource.MustParse("4"),
								},
								Limits: api.ResourceList{
									api.ResourceCPU:                        resource.MustParse("8"),
									api.ResourceMemory:                     resource.MustParse("200M"),
									api.ResourceName("example.com/dongle"): resource.MustParse("4"),
								},
							},
						},
						{
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									api.ResourceCPU:                        resource.MustParse("1"),
									api.ResourceMemory:                     resource.MustParse("50M"),
									api.ResourceName("example.com/dongle"): resource.MustParse("2"),
								},
								Limits: api.ResourceList{
									api.ResourceCPU:                        resource.MustParse("2"),
									api.ResourceMemory:                     resource.MustParse("100M"),
									api.ResourceName("example.com/dongle"): resource.MustParse("2"),
								},
							},
						},
					},
					Containers: []api.Container{
						{
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									api.ResourceCPU:                        resource.MustParse("1"),
									api.ResourceMemory:                     resource.MustParse("50M"),
									api.ResourceName("example.com/dongle"): resource.MustParse("1"),
								},
								Limits: api.ResourceList{
									api.ResourceCPU:                        resource.MustParse("2"),
									api.ResourceMemory:                     resource.MustParse("100M"),
									api.ResourceName("example.com/dongle"): resource.MustParse("1"),
								},
							},
						},
						{
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									api.ResourceCPU:                        resource.MustParse("2"),
									api.ResourceMemory:                     resource.MustParse("25M"),
									api.ResourceName("example.com/dongle"): resource.MustParse("2"),
								},
								Limits: api.ResourceList{
									api.ResourceCPU:                        resource.MustParse("5"),
									api.ResourceMemory:                     resource.MustParse("50M"),
									api.ResourceName("example.com/dongle"): resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceRequestsCPU:                         resource.MustParse("4"),
				corev1.ResourceRequestsMemory:                      resource.MustParse("100M"),
				corev1.ResourceLimitsCPU:                           resource.MustParse("8"),
				corev1.ResourceLimitsMemory:                        resource.MustParse("200M"),
				corev1.ResourcePods:                                resource.MustParse("1"),
				corev1.ResourceCPU:                                 resource.MustParse("4"),
				corev1.ResourceMemory:                              resource.MustParse("100M"),
				corev1.ResourceName("requests.example.com/dongle"): resource.MustParse("4"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"pod deletion timestamp exceeded": {
			pod: &api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp:          &deletionTimestampPastGracePeriod,
					DeletionGracePeriodSeconds: &terminationGracePeriodSeconds,
				},
				Status: api.PodStatus{
					Reason: node.NodeUnreachablePodReason,
				},
				Spec: api.PodSpec{
					TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
					Containers: []api.Container{
						{
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									api.ResourceCPU:    resource.MustParse("1"),
									api.ResourceMemory: resource.MustParse("50M"),
								},
								Limits: api.ResourceList{
									api.ResourceCPU:    resource.MustParse("2"),
									api.ResourceMemory: resource.MustParse("100M"),
								},
							},
						},
					},
				},
			},
			usage: corev1.ResourceList{
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"pod deletion timestamp not exceeded": {
			pod: &api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp:          &deletionTimestampNotPastGracePeriod,
					DeletionGracePeriodSeconds: &terminationGracePeriodSeconds,
				},
				Status: api.PodStatus{
					Reason: node.NodeUnreachablePodReason,
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						{
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									api.ResourceCPU: resource.MustParse("1"),
								},
								Limits: api.ResourceList{
									api.ResourceCPU: resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("1"),
				corev1.ResourceLimitsCPU:   resource.MustParse("2"),
				corev1.ResourcePods:        resource.MustParse("1"),
				corev1.ResourceCPU:         resource.MustParse("1"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
		},
		"count pod overhead as usage": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Overhead: api.ResourceList{
						api.ResourceCPU: resource.MustParse("1"),
					},
					Containers: []api.Container{
						{
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									api.ResourceCPU: resource.MustParse("1"),
								},
								Limits: api.ResourceList{
									api.ResourceCPU: resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("2"),
				corev1.ResourceLimitsCPU:   resource.MustParse("3"),
				corev1.ResourcePods:        resource.MustParse("1"),
				corev1.ResourceCPU:         resource.MustParse("2"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
			podOverheadEnabled: true,
		},
		"do not count pod overhead as usage with pod overhead disabled": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Overhead: api.ResourceList{
						api.ResourceCPU: resource.MustParse("1"),
					},
					Containers: []api.Container{
						{
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									api.ResourceCPU: resource.MustParse("1"),
								},
								Limits: api.ResourceList{
									api.ResourceCPU: resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("1"),
				corev1.ResourceLimitsCPU:   resource.MustParse("2"),
				corev1.ResourcePods:        resource.MustParse("1"),
				corev1.ResourceCPU:         resource.MustParse("1"),
				generic.ObjectCountQuotaResourceNameFor(schema.GroupResource{Resource: "pods"}): resource.MustParse("1"),
			},
			podOverheadEnabled: false,
		},
	}
	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodOverhead, testCase.podOverheadEnabled)()
			actual, err := evaluator.Usage(testCase.pod)
			if err != nil {
				t.Error(err)
			}
			if !quota.Equals(testCase.usage, actual) {
				t.Errorf("expected: %v, actual: %v", testCase.usage, actual)
			}
		})
	}
}

func TestPodEvaluatorMatchingScopes(t *testing.T) {
	fakeClock := clock.NewFakeClock(time.Now())
	evaluator := NewPodEvaluator(nil, fakeClock)
	activeDeadlineSeconds := int64(30)
	testCases := map[string]struct {
		pod                      *api.Pod
		selectors                []corev1.ScopedResourceSelectorRequirement
		wantSelectors            []corev1.ScopedResourceSelectorRequirement
		disableNamespaceSelector bool
	}{
		"EmptyPod": {
			pod: &api.Pod{},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeNotTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
			},
		},
		"PriorityClass": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					PriorityClassName: "class1",
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeNotTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
				{ScopeName: corev1.ResourceQuotaScopePriorityClass, Operator: corev1.ScopeSelectorOpIn, Values: []string{"class1"}},
			},
		},
		"NotBestEffort": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{{
						Resources: api.ResourceRequirements{
							Requests: api.ResourceList{
								api.ResourceCPU:                        resource.MustParse("1"),
								api.ResourceMemory:                     resource.MustParse("50M"),
								api.ResourceName("example.com/dongle"): resource.MustParse("1"),
							},
							Limits: api.ResourceList{
								api.ResourceCPU:                        resource.MustParse("2"),
								api.ResourceMemory:                     resource.MustParse("100M"),
								api.ResourceName("example.com/dongle"): resource.MustParse("1"),
							},
						},
					}},
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeNotTerminating},
				{ScopeName: corev1.ResourceQuotaScopeNotBestEffort},
			},
		},
		"Terminating": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
			},
		},
		"OnlyTerminating": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
				},
			},
			selectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
			},
		},
		"CrossNamespaceRequiredAffinity": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
					Affinity: &api.Affinity{
						PodAffinity: &api.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []api.PodAffinityTerm{
								{LabelSelector: &metav1.LabelSelector{}, Namespaces: []string{"ns1"}, NamespaceSelector: &metav1.LabelSelector{}},
							},
						},
					},
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
				{ScopeName: corev1.ResourceQuotaScopeCrossNamespacePodAffinity},
			},
		},
		"CrossNamespaceRequiredAffinityWithSlice": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
					Affinity: &api.Affinity{
						PodAffinity: &api.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []api.PodAffinityTerm{
								{LabelSelector: &metav1.LabelSelector{}, Namespaces: []string{"ns1"}},
							},
						},
					},
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
				{ScopeName: corev1.ResourceQuotaScopeCrossNamespacePodAffinity},
			},
		},
		"CrossNamespacePreferredAffinity": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
					Affinity: &api.Affinity{
						PodAffinity: &api.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []api.WeightedPodAffinityTerm{
								{PodAffinityTerm: api.PodAffinityTerm{LabelSelector: &metav1.LabelSelector{}, Namespaces: []string{"ns2"}, NamespaceSelector: &metav1.LabelSelector{}}},
							},
						},
					},
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
				{ScopeName: corev1.ResourceQuotaScopeCrossNamespacePodAffinity},
			},
		},
		"CrossNamespacePreferredAffinityWithSelector": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
					Affinity: &api.Affinity{
						PodAffinity: &api.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []api.WeightedPodAffinityTerm{
								{PodAffinityTerm: api.PodAffinityTerm{LabelSelector: &metav1.LabelSelector{}, NamespaceSelector: &metav1.LabelSelector{}}},
							},
						},
					},
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
				{ScopeName: corev1.ResourceQuotaScopeCrossNamespacePodAffinity},
			},
		},
		"CrossNamespacePreferredAntiAffinity": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
					Affinity: &api.Affinity{
						PodAntiAffinity: &api.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []api.WeightedPodAffinityTerm{
								{PodAffinityTerm: api.PodAffinityTerm{LabelSelector: &metav1.LabelSelector{}, NamespaceSelector: &metav1.LabelSelector{}}},
							},
						},
					},
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
				{ScopeName: corev1.ResourceQuotaScopeCrossNamespacePodAffinity},
			},
		},
		"CrossNamespaceRequiredAntiAffinity": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
					Affinity: &api.Affinity{
						PodAntiAffinity: &api.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []api.PodAffinityTerm{
								{LabelSelector: &metav1.LabelSelector{}, Namespaces: []string{"ns3"}},
							},
						},
					},
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
				{ScopeName: corev1.ResourceQuotaScopeCrossNamespacePodAffinity},
			},
		},
		"NamespaceSelectorFeatureDisabled": {
			pod: &api.Pod{
				Spec: api.PodSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
					Affinity: &api.Affinity{
						PodAntiAffinity: &api.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []api.PodAffinityTerm{
								{LabelSelector: &metav1.LabelSelector{}, Namespaces: []string{"ns3"}},
							},
						},
					},
				},
			},
			wantSelectors: []corev1.ScopedResourceSelectorRequirement{
				{ScopeName: corev1.ResourceQuotaScopeTerminating},
				{ScopeName: corev1.ResourceQuotaScopeBestEffort},
			},
			disableNamespaceSelector: true,
		},
	}
	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodAffinityNamespaceSelector, !testCase.disableNamespaceSelector)()
			if testCase.selectors == nil {
				testCase.selectors = []corev1.ScopedResourceSelectorRequirement{
					{ScopeName: corev1.ResourceQuotaScopeTerminating},
					{ScopeName: corev1.ResourceQuotaScopeNotTerminating},
					{ScopeName: corev1.ResourceQuotaScopeBestEffort},
					{ScopeName: corev1.ResourceQuotaScopeNotBestEffort},
					{ScopeName: corev1.ResourceQuotaScopePriorityClass, Operator: corev1.ScopeSelectorOpIn, Values: []string{"class1"}},
					{ScopeName: corev1.ResourceQuotaScopeCrossNamespacePodAffinity},
				}
			}
			gotSelectors, err := evaluator.MatchingScopes(testCase.pod, testCase.selectors)
			if err != nil {
				t.Error(err)
			}
			if diff := cmp.Diff(testCase.wantSelectors, gotSelectors); diff != "" {
				t.Errorf("%v: unexpected diff (-want, +got):\n%s", testName, diff)
			}
		})
	}
}
