/*
Copyright 2022 The Kubernetes Authors.

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

package v1

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/util/feature"
	componentbaseconfig "k8s.io/component-base/config/v1alpha1"
	"k8s.io/component-base/featuregate"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	configv1 "k8s.io/kube-scheduler/config/v1"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/names"
	"k8s.io/utils/pointer"
)

var pluginConfigs = []configv1.PluginConfig{
	{
		Name: "DefaultPreemption",
		Args: runtime.RawExtension{
			Object: &configv1.DefaultPreemptionArgs{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DefaultPreemptionArgs",
					APIVersion: "kubescheduler.config.k8s.io/v1",
				},
				MinCandidateNodesPercentage: pointer.Int32Ptr(10),
				MinCandidateNodesAbsolute:   pointer.Int32Ptr(100),
			}},
	},
	{
		Name: "InterPodAffinity",
		Args: runtime.RawExtension{
			Object: &configv1.InterPodAffinityArgs{
				TypeMeta: metav1.TypeMeta{
					Kind:       "InterPodAffinityArgs",
					APIVersion: "kubescheduler.config.k8s.io/v1",
				},
				HardPodAffinityWeight: pointer.Int32Ptr(1),
			}},
	},
	{
		Name: "NodeAffinity",
		Args: runtime.RawExtension{Object: &configv1.NodeAffinityArgs{
			TypeMeta: metav1.TypeMeta{
				Kind:       "NodeAffinityArgs",
				APIVersion: "kubescheduler.config.k8s.io/v1",
			},
		}},
	},
	{
		Name: "NodeResourcesBalancedAllocation",
		Args: runtime.RawExtension{Object: &configv1.NodeResourcesBalancedAllocationArgs{
			TypeMeta: metav1.TypeMeta{
				Kind:       "NodeResourcesBalancedAllocationArgs",
				APIVersion: "kubescheduler.config.k8s.io/v1",
			},
			Resources: []configv1.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
		}},
	},
	{
		Name: "NodeResourcesFit",
		Args: runtime.RawExtension{Object: &configv1.NodeResourcesFitArgs{
			TypeMeta: metav1.TypeMeta{
				Kind:       "NodeResourcesFitArgs",
				APIVersion: "kubescheduler.config.k8s.io/v1",
			},
			ScoringStrategy: &configv1.ScoringStrategy{
				Type:      configv1.LeastAllocated,
				Resources: []configv1.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
			},
		}},
	},
	{
		Name: "PodTopologySpread",
		Args: runtime.RawExtension{Object: &configv1.PodTopologySpreadArgs{
			TypeMeta: metav1.TypeMeta{
				Kind:       "PodTopologySpreadArgs",
				APIVersion: "kubescheduler.config.k8s.io/v1",
			},
			DefaultingType: configv1.SystemDefaulting,
		}},
	},
	{
		Name: "VolumeBinding",
		Args: runtime.RawExtension{Object: &configv1.VolumeBindingArgs{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VolumeBindingArgs",
				APIVersion: "kubescheduler.config.k8s.io/v1",
			},
			BindTimeoutSeconds: pointer.Int64Ptr(600),
		}},
	},
}

func TestSchedulerDefaults(t *testing.T) {
	enable := true
	tests := []struct {
		name     string
		config   *configv1.KubeSchedulerConfiguration
		expected *configv1.KubeSchedulerConfiguration
	}{
		{
			name:   "empty config",
			config: &configv1.KubeSchedulerConfiguration{},
			expected: &configv1.KubeSchedulerConfiguration{
				Parallelism: pointer.Int32Ptr(16),
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           &enable,
					EnableContentionProfiling: &enable,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       pointer.BoolPtr(true),
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: pointer.Int32Ptr(0),
				PodInitialBackoffSeconds: pointer.Int64Ptr(1),
				PodMaxBackoffSeconds:     pointer.Int64Ptr(10),
				Profiles: []configv1.KubeSchedulerProfile{
					{
						Plugins:       getDefaultPlugins(),
						PluginConfig:  pluginConfigs,
						SchedulerName: pointer.StringPtr("default-scheduler"),
					},
				},
			},
		},
		{
			name: "no scheduler name",
			config: &configv1.KubeSchedulerConfiguration{
				Profiles: []configv1.KubeSchedulerProfile{{}},
			},
			expected: &configv1.KubeSchedulerConfiguration{
				Parallelism: pointer.Int32Ptr(16),
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           &enable,
					EnableContentionProfiling: &enable,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       pointer.BoolPtr(true),
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: pointer.Int32Ptr(0),
				PodInitialBackoffSeconds: pointer.Int64Ptr(1),
				PodMaxBackoffSeconds:     pointer.Int64Ptr(10),
				Profiles: []configv1.KubeSchedulerProfile{
					{
						SchedulerName: pointer.StringPtr("default-scheduler"),
						Plugins:       getDefaultPlugins(),
						PluginConfig:  pluginConfigs},
				},
			},
		},
		{
			name: "two profiles",
			config: &configv1.KubeSchedulerConfiguration{
				Parallelism: pointer.Int32Ptr(16),
				Profiles: []configv1.KubeSchedulerProfile{
					{
						PluginConfig: []configv1.PluginConfig{
							{Name: "FooPlugin"},
						},
					},
					{
						SchedulerName: pointer.StringPtr("custom-scheduler"),
						Plugins: &configv1.Plugins{
							Bind: configv1.PluginSet{
								Enabled: []configv1.Plugin{
									{Name: "BarPlugin"},
								},
								Disabled: []configv1.Plugin{
									{Name: names.DefaultBinder},
								},
							},
						},
					},
				},
			},
			expected: &configv1.KubeSchedulerConfiguration{
				Parallelism: pointer.Int32Ptr(16),
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           &enable,
					EnableContentionProfiling: &enable,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       pointer.BoolPtr(true),
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: pointer.Int32Ptr(0),
				PodInitialBackoffSeconds: pointer.Int64Ptr(1),
				PodMaxBackoffSeconds:     pointer.Int64Ptr(10),
				Profiles: []configv1.KubeSchedulerProfile{
					{
						Plugins: getDefaultPlugins(),
						PluginConfig: []configv1.PluginConfig{
							{Name: "FooPlugin"},
							{
								Name: "DefaultPreemption",
								Args: runtime.RawExtension{
									Object: &configv1.DefaultPreemptionArgs{
										TypeMeta: metav1.TypeMeta{
											Kind:       "DefaultPreemptionArgs",
											APIVersion: "kubescheduler.config.k8s.io/v1",
										},
										MinCandidateNodesPercentage: pointer.Int32Ptr(10),
										MinCandidateNodesAbsolute:   pointer.Int32Ptr(100),
									}},
							},
							{
								Name: "InterPodAffinity",
								Args: runtime.RawExtension{
									Object: &configv1.InterPodAffinityArgs{
										TypeMeta: metav1.TypeMeta{
											Kind:       "InterPodAffinityArgs",
											APIVersion: "kubescheduler.config.k8s.io/v1",
										},
										HardPodAffinityWeight: pointer.Int32Ptr(1),
									}},
							},
							{
								Name: "NodeAffinity",
								Args: runtime.RawExtension{Object: &configv1.NodeAffinityArgs{
									TypeMeta: metav1.TypeMeta{
										Kind:       "NodeAffinityArgs",
										APIVersion: "kubescheduler.config.k8s.io/v1",
									},
								}},
							},
							{
								Name: "NodeResourcesBalancedAllocation",
								Args: runtime.RawExtension{Object: &configv1.NodeResourcesBalancedAllocationArgs{
									TypeMeta: metav1.TypeMeta{
										Kind:       "NodeResourcesBalancedAllocationArgs",
										APIVersion: "kubescheduler.config.k8s.io/v1",
									},
									Resources: []configv1.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
								}},
							},
							{
								Name: "NodeResourcesFit",
								Args: runtime.RawExtension{Object: &configv1.NodeResourcesFitArgs{
									TypeMeta: metav1.TypeMeta{
										Kind:       "NodeResourcesFitArgs",
										APIVersion: "kubescheduler.config.k8s.io/v1",
									},
									ScoringStrategy: &configv1.ScoringStrategy{
										Type:      configv1.LeastAllocated,
										Resources: []configv1.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
									},
								}},
							},
							{
								Name: "PodTopologySpread",
								Args: runtime.RawExtension{Object: &configv1.PodTopologySpreadArgs{
									TypeMeta: metav1.TypeMeta{
										Kind:       "PodTopologySpreadArgs",
										APIVersion: "kubescheduler.config.k8s.io/v1",
									},
									DefaultingType: configv1.SystemDefaulting,
								}},
							},
							{
								Name: "VolumeBinding",
								Args: runtime.RawExtension{Object: &configv1.VolumeBindingArgs{
									TypeMeta: metav1.TypeMeta{
										Kind:       "VolumeBindingArgs",
										APIVersion: "kubescheduler.config.k8s.io/v1",
									},
									BindTimeoutSeconds: pointer.Int64Ptr(600),
								}},
							},
						},
					},
					{
						SchedulerName: pointer.StringPtr("custom-scheduler"),
						Plugins: &configv1.Plugins{
							MultiPoint: configv1.PluginSet{
								Enabled: []configv1.Plugin{
									{Name: names.PrioritySort},
									{Name: names.NodeUnschedulable},
									{Name: names.NodeName},
									{Name: names.TaintToleration, Weight: pointer.Int32(3)},
									{Name: names.NodeAffinity, Weight: pointer.Int32(2)},
									{Name: names.NodePorts},
									{Name: names.NodeResourcesFit, Weight: pointer.Int32(1)},
									{Name: names.VolumeRestrictions},
									{Name: names.EBSLimits},
									{Name: names.GCEPDLimits},
									{Name: names.NodeVolumeLimits},
									{Name: names.AzureDiskLimits},
									{Name: names.VolumeBinding},
									{Name: names.VolumeZone},
									{Name: names.PodTopologySpread, Weight: pointer.Int32(2)},
									{Name: names.InterPodAffinity, Weight: pointer.Int32(2)},
									{Name: names.DefaultPreemption},
									{Name: names.NodeResourcesBalancedAllocation, Weight: pointer.Int32(1)},
									{Name: names.ImageLocality, Weight: pointer.Int32(1)},
									{Name: names.DefaultBinder},
								},
							},
							Bind: configv1.PluginSet{
								Enabled: []configv1.Plugin{
									{Name: "BarPlugin"},
								},
								Disabled: []configv1.Plugin{
									{Name: names.DefaultBinder},
								},
							},
						},
						PluginConfig: pluginConfigs,
					},
				},
			},
		},
		{
			name: "Prallelism with no port",
			config: &configv1.KubeSchedulerConfiguration{
				Parallelism: pointer.Int32Ptr(16),
			},
			expected: &configv1.KubeSchedulerConfiguration{
				Parallelism: pointer.Int32Ptr(16),
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           &enable,
					EnableContentionProfiling: &enable,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       pointer.BoolPtr(true),
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: pointer.Int32Ptr(0),
				PodInitialBackoffSeconds: pointer.Int64Ptr(1),
				PodMaxBackoffSeconds:     pointer.Int64Ptr(10),
				Profiles: []configv1.KubeSchedulerProfile{
					{
						Plugins:       getDefaultPlugins(),
						PluginConfig:  pluginConfigs,
						SchedulerName: pointer.StringPtr("default-scheduler"),
					},
				},
			},
		},
		{
			name: "set non default parallelism",
			config: &configv1.KubeSchedulerConfiguration{
				Parallelism: pointer.Int32Ptr(8),
			},
			expected: &configv1.KubeSchedulerConfiguration{
				Parallelism: pointer.Int32Ptr(8),
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           &enable,
					EnableContentionProfiling: &enable,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       pointer.BoolPtr(true),
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: pointer.Int32Ptr(0),
				PodInitialBackoffSeconds: pointer.Int64Ptr(1),
				PodMaxBackoffSeconds:     pointer.Int64Ptr(10),
				Profiles: []configv1.KubeSchedulerProfile{
					{
						Plugins:       getDefaultPlugins(),
						PluginConfig:  pluginConfigs,
						SchedulerName: pointer.StringPtr("default-scheduler"),
					},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			SetDefaults_KubeSchedulerConfiguration(tc.config)
			if diff := cmp.Diff(tc.expected, tc.config); diff != "" {
				t.Errorf("Got unexpected defaults (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestPluginArgsDefaults(t *testing.T) {
	tests := []struct {
		name     string
		features map[featuregate.Feature]bool
		in       runtime.Object
		want     runtime.Object
	}{
		{
			name: "DefaultPreemptionArgs empty",
			in:   &configv1.DefaultPreemptionArgs{},
			want: &configv1.DefaultPreemptionArgs{
				MinCandidateNodesPercentage: pointer.Int32Ptr(10),
				MinCandidateNodesAbsolute:   pointer.Int32Ptr(100),
			},
		},
		{
			name: "DefaultPreemptionArgs with value",
			in: &configv1.DefaultPreemptionArgs{
				MinCandidateNodesPercentage: pointer.Int32Ptr(50),
			},
			want: &configv1.DefaultPreemptionArgs{
				MinCandidateNodesPercentage: pointer.Int32Ptr(50),
				MinCandidateNodesAbsolute:   pointer.Int32Ptr(100),
			},
		},
		{
			name: "InterPodAffinityArgs empty",
			in:   &configv1.InterPodAffinityArgs{},
			want: &configv1.InterPodAffinityArgs{
				HardPodAffinityWeight: pointer.Int32Ptr(1),
			},
		},
		{
			name: "InterPodAffinityArgs explicit 0",
			in: &configv1.InterPodAffinityArgs{
				HardPodAffinityWeight: pointer.Int32Ptr(0),
			},
			want: &configv1.InterPodAffinityArgs{
				HardPodAffinityWeight: pointer.Int32Ptr(0),
			},
		},
		{
			name: "InterPodAffinityArgs with value",
			in: &configv1.InterPodAffinityArgs{
				HardPodAffinityWeight: pointer.Int32Ptr(5),
			},
			want: &configv1.InterPodAffinityArgs{
				HardPodAffinityWeight: pointer.Int32Ptr(5),
			},
		},
		{
			name: "NodeResourcesBalancedAllocationArgs resources empty",
			in:   &configv1.NodeResourcesBalancedAllocationArgs{},
			want: &configv1.NodeResourcesBalancedAllocationArgs{
				Resources: []configv1.ResourceSpec{
					{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1},
				},
			},
		},
		{
			name: "NodeResourcesBalancedAllocationArgs with scalar resource",
			in: &configv1.NodeResourcesBalancedAllocationArgs{
				Resources: []configv1.ResourceSpec{
					{Name: "scalar.io/scalar1", Weight: 1},
				},
			},
			want: &configv1.NodeResourcesBalancedAllocationArgs{
				Resources: []configv1.ResourceSpec{
					{Name: "scalar.io/scalar1", Weight: 1},
				},
			},
		},
		{
			name: "NodeResourcesBalancedAllocationArgs with mixed resources",
			in: &configv1.NodeResourcesBalancedAllocationArgs{
				Resources: []configv1.ResourceSpec{
					{Name: string(v1.ResourceCPU), Weight: 1},
					{Name: "scalar.io/scalar1", Weight: 1},
				},
			},
			want: &configv1.NodeResourcesBalancedAllocationArgs{
				Resources: []configv1.ResourceSpec{
					{Name: string(v1.ResourceCPU), Weight: 1},
					{Name: "scalar.io/scalar1", Weight: 1},
				},
			},
		},
		{
			name: "NodeResourcesBalancedAllocationArgs have resource no weight",
			in: &configv1.NodeResourcesBalancedAllocationArgs{
				Resources: []configv1.ResourceSpec{
					{Name: string(v1.ResourceCPU)},
					{Name: "scalar.io/scalar0"},
					{Name: "scalar.io/scalar1", Weight: 1},
				},
			},
			want: &configv1.NodeResourcesBalancedAllocationArgs{
				Resources: []configv1.ResourceSpec{
					{Name: string(v1.ResourceCPU), Weight: 1},
					{Name: "scalar.io/scalar0", Weight: 1},
					{Name: "scalar.io/scalar1", Weight: 1},
				},
			},
		},
		{
			name: "PodTopologySpreadArgs resources empty",
			in:   &configv1.PodTopologySpreadArgs{},
			want: &configv1.PodTopologySpreadArgs{
				DefaultingType: configv1.SystemDefaulting,
			},
		},
		{
			name: "PodTopologySpreadArgs resources with value",
			in: &configv1.PodTopologySpreadArgs{
				DefaultConstraints: []v1.TopologySpreadConstraint{
					{
						TopologyKey:       "planet",
						WhenUnsatisfiable: v1.DoNotSchedule,
						MaxSkew:           2,
					},
				},
			},
			want: &configv1.PodTopologySpreadArgs{
				DefaultConstraints: []v1.TopologySpreadConstraint{
					{
						TopologyKey:       "planet",
						WhenUnsatisfiable: v1.DoNotSchedule,
						MaxSkew:           2,
					},
				},
				DefaultingType: configv1.SystemDefaulting,
			},
		},
		{
			name: "NodeResourcesFitArgs not set",
			in:   &configv1.NodeResourcesFitArgs{},
			want: &configv1.NodeResourcesFitArgs{
				ScoringStrategy: &configv1.ScoringStrategy{
					Type:      configv1.LeastAllocated,
					Resources: defaultResourceSpec,
				},
			},
		},
		{
			name: "NodeResourcesFitArgs Resources empty",
			in: &configv1.NodeResourcesFitArgs{
				ScoringStrategy: &configv1.ScoringStrategy{
					Type: configv1.MostAllocated,
				},
			},
			want: &configv1.NodeResourcesFitArgs{
				ScoringStrategy: &configv1.ScoringStrategy{
					Type:      configv1.MostAllocated,
					Resources: defaultResourceSpec,
				},
			},
		},
		{
			name: "VolumeBindingArgs empty, VolumeCapacityPriority disabled",
			features: map[featuregate.Feature]bool{
				features.VolumeCapacityPriority: false,
			},
			in: &configv1.VolumeBindingArgs{},
			want: &configv1.VolumeBindingArgs{
				BindTimeoutSeconds: pointer.Int64Ptr(600),
			},
		},
		{
			name: "VolumeBindingArgs empty, VolumeCapacityPriority enabled",
			features: map[featuregate.Feature]bool{
				features.VolumeCapacityPriority: true,
			},
			in: &configv1.VolumeBindingArgs{},
			want: &configv1.VolumeBindingArgs{
				BindTimeoutSeconds: pointer.Int64Ptr(600),
				Shape: []configv1.UtilizationShapePoint{
					{Utilization: 0, Score: 0},
					{Utilization: 100, Score: 10},
				},
			},
		},
	}
	for _, tc := range tests {
		scheme := runtime.NewScheme()
		utilruntime.Must(AddToScheme(scheme))
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.features {
				defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, k, v)()
			}
			scheme.Default(tc.in)
			if diff := cmp.Diff(tc.in, tc.want); diff != "" {
				t.Errorf("Got unexpected defaults (-want, +got):\n%s", diff)
			}
		})
	}
}
