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

package config

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestKubeletConfigurationPathFields(t *testing.T) {
	// ensure the intersection of kubeletConfigurationPathFieldPaths and KubeletConfigurationNonPathFields is empty
	if i := kubeletConfigurationPathFieldPaths.Intersection(kubeletConfigurationNonPathFieldPaths); len(i) > 0 {
		t.Fatalf("expect the intersection of kubeletConfigurationPathFieldPaths and "+
			"KubeletConfigurationNonPathFields to be empty, got:\n%s",
			strings.Join(i.List(), "\n"))
	}

	// ensure that kubeletConfigurationPathFields U kubeletConfigurationNonPathFields == allPrimitiveFieldPaths(KubeletConfiguration)
	expect := sets.NewString().Union(kubeletConfigurationPathFieldPaths).Union(kubeletConfigurationNonPathFieldPaths)
	result := allPrimitiveFieldPaths(t, expect, reflect.TypeOf(&KubeletConfiguration{}), nil)
	if !expect.Equal(result) {
		// expected fields missing from result
		missing := expect.Difference(result)
		// unexpected fields in result but not specified in expect
		unexpected := result.Difference(expect)
		if len(missing) > 0 {
			t.Errorf("the following fields were expected, but missing from the result. "+
				"If the field has been removed, please remove it from the kubeletConfigurationPathFieldPaths set "+
				"and the KubeletConfigurationPathRefs function, "+
				"or remove it from the kubeletConfigurationNonPathFieldPaths set, as appropriate:\n%s",
				strings.Join(missing.List(), "\n"))
		}
		if len(unexpected) > 0 {
			t.Errorf("the following fields were in the result, but unexpected. "+
				"If the field is new, please add it to the kubeletConfigurationPathFieldPaths set "+
				"and the KubeletConfigurationPathRefs function, "+
				"or add it to the kubeletConfigurationNonPathFieldPaths set, as appropriate:\n%s",
				strings.Join(unexpected.List(), "\n"))
		}
	}
}

// allPrimitiveFieldPaths returns the set of field paths in type `tp`, rooted at `path`.
// It recursively descends into the definition of type `tp` accumulating paths to primitive leaf fields or paths in `skipRecurseList`.
func allPrimitiveFieldPaths(t *testing.T, skipRecurseList sets.String, tp reflect.Type, path *field.Path) sets.String {
	// if the current field path is in the list of paths we should not recurse into,
	// return here rather than descending and accumulating child field paths
	if pathStr := path.String(); len(pathStr) > 0 && skipRecurseList.Has(pathStr) {
		return sets.NewString(pathStr)
	}

	paths := sets.NewString()
	switch tp.Kind() {
	case reflect.Pointer:
		paths.Insert(allPrimitiveFieldPaths(t, skipRecurseList, tp.Elem(), path).List()...)
	case reflect.Struct:
		for i := 0; i < tp.NumField(); i++ {
			field := tp.Field(i)
			paths.Insert(allPrimitiveFieldPaths(t, skipRecurseList, field.Type, path.Child(field.Name)).List()...)
		}
	case reflect.Map, reflect.Slice:
		paths.Insert(allPrimitiveFieldPaths(t, skipRecurseList, tp.Elem(), path.Key("*")).List()...)
	case reflect.Interface:
		t.Fatalf("unexpected interface{} field %s", path.String())
	default:
		// if we hit a primitive type, we're at a leaf
		paths.Insert(path.String())
	}
	return paths
}

//lint:file-ignore U1000 Ignore dummy types, used by tests.

// dummy helper types
type foo struct {
	foo int
}
type bar struct {
	str    string
	strptr *string

	ints      []int
	stringMap map[string]string

	foo    foo
	fooptr *foo

	bars   []foo
	barMap map[string]foo

	skipRecurseStruct  foo
	skipRecursePointer *foo
	skipRecurseList1   []foo
	skipRecurseList2   []foo
	skipRecurseMap1    map[string]foo
	skipRecurseMap2    map[string]foo
}

func TestAllPrimitiveFieldPaths(t *testing.T) {
	expect := sets.NewString(
		"str",
		"strptr",
		"ints[*]",
		"stringMap[*]",
		"foo.foo",
		"fooptr.foo",
		"bars[*].foo",
		"barMap[*].foo",
		"skipRecurseStruct",   // skip recursing a struct
		"skipRecursePointer",  // skip recursing a struct pointer
		"skipRecurseList1",    // skip recursing a list
		"skipRecurseList2[*]", // skip recursing list items
		"skipRecurseMap1",     // skip recursing a map
		"skipRecurseMap2[*]",  // skip recursing map items
	)
	result := allPrimitiveFieldPaths(t, expect, reflect.TypeOf(&bar{}), nil)
	if !expect.Equal(result) {
		// expected fields missing from result
		missing := expect.Difference(result)

		// unexpected fields in result but not specified in expect
		unexpected := result.Difference(expect)

		if len(missing) > 0 {
			t.Errorf("the following fields were expected, but missing from the result:\n%s", strings.Join(missing.List(), "\n"))
		}
		if len(unexpected) > 0 {
			t.Errorf("the following fields were in the result, but unexpected:\n%s", strings.Join(unexpected.List(), "\n"))
		}
	}
}

var (
	// KubeletConfiguration fields that contain file paths. If you update this, also update KubeletConfigurationPathRefs!
	kubeletConfigurationPathFieldPaths = sets.NewString(
		"StaticPodPath",
		"Authentication.X509.ClientCAFile",
		"TLSCertFile",
		"TLSPrivateKeyFile",
		"ResolverConfig",
	)

	// KubeletConfiguration fields that do not contain file paths.
	kubeletConfigurationNonPathFieldPaths = sets.NewString(
		"Address",
		"AllowedUnsafeSysctls[*]",
		"Authentication.Anonymous.Enabled",
		"Authentication.Webhook.CacheTTL.Duration",
		"Authentication.Webhook.Enabled",
		"Authorization.Mode",
		"Authorization.Webhook.CacheAuthorizedTTL.Duration",
		"Authorization.Webhook.CacheUnauthorizedTTL.Duration",
		"CPUCFSQuota",
		"CPUCFSQuotaPeriod.Duration",
		"CPUManagerPolicy",
		"CPUManagerPolicyOptions[*]",
		"CPUManagerReconcilePeriod.Duration",
		"TopologyManagerPolicy",
		"TopologyManagerScope",
		"QOSReserved[*]",
		"CgroupDriver",
		"CgroupRoot",
		"CgroupsPerQOS",
		"ClusterDNS[*]",
		"ClusterDomain",
		"ConfigMapAndSecretChangeDetectionStrategy",
		"ContainerLogMaxFiles",
		"ContainerLogMaxSize",
		"ContentType",
		"EnableContentionProfiling",
		"EnableControllerAttachDetach",
		"EnableDebugFlagsHandler",
		"EnableDebuggingHandlers",
		"EnableProfilingHandler",
		"EnableServer",
		"EnableSystemLogHandler",
		"EnforceNodeAllocatable[*]",
		"EventBurst",
		"EventRecordQPS",
		"EvictionHard[*]",
		"EvictionMaxPodGracePeriod",
		"EvictionMinimumReclaim[*]",
		"EvictionPressureTransitionPeriod.Duration",
		"EvictionSoft[*]",
		"EvictionSoftGracePeriod[*]",
		"FailSwapOn",
		"FeatureGates[*]",
		"FileCheckFrequency.Duration",
		"HTTPCheckFrequency.Duration",
		"HairpinMode",
		"HealthzBindAddress",
		"HealthzPort",
		"Logging.FlushFrequency",
		"Logging.Format",
		"Logging.Options.JSON.InfoBufferSize.Quantity.Format",
		"Logging.Options.JSON.InfoBufferSize.Quantity.d.Dec.scale",
		"Logging.Options.JSON.InfoBufferSize.Quantity.d.Dec.unscaled.abs[*]",
		"Logging.Options.JSON.InfoBufferSize.Quantity.d.Dec.unscaled.neg",
		"Logging.Options.JSON.InfoBufferSize.Quantity.i.scale",
		"Logging.Options.JSON.InfoBufferSize.Quantity.i.value",
		"Logging.Options.JSON.InfoBufferSize.Quantity.s",
		"Logging.Options.JSON.SplitStream",
		"Logging.VModule[*].FilePattern",
		"Logging.VModule[*].Verbosity",
		"Logging.Verbosity",
		"TLSCipherSuites[*]",
		"TLSMinVersion",
		"IPTablesDropBit",
		"IPTablesMasqueradeBit",
		"ImageGCHighThresholdPercent",
		"ImageGCLowThresholdPercent",
		"ImageMinimumGCAge.Duration",
		"KernelMemcgNotification",
		"KubeAPIBurst",
		"KubeAPIQPS",
		"KubeReservedCgroup",
		"KubeReserved[*]",
		"KubeletCgroups",
		"MakeIPTablesUtilChains",
		"RotateCertificates",
		"ServerTLSBootstrap",
		"StaticPodURL",
		"StaticPodURLHeader[*][*]",
		"MaxOpenFiles",
		"MaxPods",
		"MemoryManagerPolicy",
		"MemorySwap.SwapBehavior",
		"NodeLeaseDurationSeconds",
		"NodeStatusMaxImages",
		"NodeStatusUpdateFrequency.Duration",
		"NodeStatusReportFrequency.Duration",
		"OOMScoreAdj",
		"PodCIDR",
		"PodPidsLimit",
		"PodsPerCore",
		"Port",
		"ProtectKernelDefaults",
		"ProviderID",
		"ReadOnlyPort",
		"RegisterNode",
		"RegistryBurst",
		"RegistryPullQPS",
		"ReservedMemory",
		"ReservedSystemCPUs",
		"RegisterWithTaints",
		"RuntimeRequestTimeout.Duration",
		"RunOnce",
		"SeccompDefault",
		"SerializeImagePulls",
		"ShowHiddenMetricsForVersion",
		"ShutdownGracePeriodByPodPriority[*].Priority",
		"ShutdownGracePeriodByPodPriority[*].ShutdownGracePeriodSeconds",
		"StreamingConnectionIdleTimeout.Duration",
		"SyncFrequency.Duration",
		"SystemCgroups",
		"SystemReservedCgroup",
		"SystemReserved[*]",
		"TypeMeta.APIVersion",
		"TypeMeta.Kind",
		"VolumeStatsAggPeriod.Duration",
		"VolumePluginDir",
		"ShutdownGracePeriod.Duration",
		"ShutdownGracePeriodCriticalPods.Duration",
		"MemoryThrottlingFactor",
		"Tracing.Endpoint",
		"Tracing.SamplingRatePerMillion",
		"LocalStorageCapacityIsolation",
	)
)
