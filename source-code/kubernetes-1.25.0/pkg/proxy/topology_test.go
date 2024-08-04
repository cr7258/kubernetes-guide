/*
Copyright 2019 The Kubernetes Authors.

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

package proxy

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
)

func checkExpectedEndpoints(expected sets.String, actual []Endpoint) error {
	var errs []error

	expectedCopy := sets.NewString(expected.UnsortedList()...)
	for _, ep := range actual {
		if !expectedCopy.Has(ep.String()) {
			errs = append(errs, fmt.Errorf("unexpected endpoint %v", ep))
		}
		expectedCopy.Delete(ep.String())
	}
	if len(expectedCopy) > 0 {
		errs = append(errs, fmt.Errorf("missing endpoints %v", expectedCopy.UnsortedList()))
	}

	return kerrors.NewAggregate(errs)
}

func TestCategorizeEndpoints(t *testing.T) {
	testCases := []struct {
		name         string
		hintsEnabled bool
		pteEnabled   bool
		nodeLabels   map[string]string
		serviceInfo  ServicePort
		endpoints    []Endpoint

		// We distinguish `nil` ("service doesn't use this kind of endpoints") from
		// `sets.String()` ("service uses this kind of endpoints but has no endpoints").
		// allEndpoints can be left unset if only one of clusterEndpoints and
		// localEndpoints is set, and allEndpoints is identical to it.
		// onlyRemoteEndpoints should be true if CategorizeEndpoints returns true for
		// hasAnyEndpoints despite allEndpoints being empty.
		clusterEndpoints    sets.String
		localEndpoints      sets.String
		allEndpoints        sets.String
		onlyRemoteEndpoints bool
	}{{
		name:         "hints enabled, hints annotation == auto",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "hints, hints annotation == disabled, hints ignored",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "disabled"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.5:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "hints, hints annotation == aUto (wrong capitalization), hints ignored",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "aUto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.5:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "hints, hints annotation empty, hints ignored",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.5:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "externalTrafficPolicy: Local, topology ignored for Local endpoints",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{externalPolicyLocal: true, nodePort: 8080, hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.6:80"),
		localEndpoints:   sets.NewString("10.1.2.3:80", "10.1.2.4:80"),
		allEndpoints:     sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.6:80"),
	}, {
		name:         "internalTrafficPolicy: Local, topology ignored for Local endpoints",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{internalPolicyLocal: true, hintsAnnotation: "auto", externalPolicyLocal: false, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.6:80"),
		localEndpoints:   sets.NewString("10.1.2.3:80", "10.1.2.4:80"),
		allEndpoints:     sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.6:80"),
	}, {
		name:         "empty node labels",
		hintsEnabled: true,
		nodeLabels:   map[string]string{},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80"),
		localEndpoints:   nil,
	}, {
		name:         "empty zone label",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: ""},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80"),
		localEndpoints:   nil,
	}, {
		name:         "node in different zone, no endpoint filtering",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-b"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80"),
		localEndpoints:   nil,
	}, {
		name:         "normal endpoint filtering, auto annotation",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "unready endpoint",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: false},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80"),
		localEndpoints:   nil,
	}, {
		name:         "only unready endpoints in same zone (should not filter)",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: false},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: false},
		},
		clusterEndpoints: sets.NewString("10.1.2.4:80", "10.1.2.5:80"),
		localEndpoints:   nil,
	}, {
		name:         "normal endpoint filtering, Auto annotation",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "Auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "hintsAnnotation empty, no filtering applied",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: ""},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.5:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "hintsAnnotation disabled, no filtering applied",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "disabled"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.5:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "missing hints, no filtering applied",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: nil, Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-a"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.5:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "multiple hints per endpoint, filtering includes any endpoint with zone included",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-c"},
		serviceInfo:  &BaseServiceInfo{hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.1.2.3:80", ZoneHints: sets.NewString("zone-a", "zone-b", "zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.4:80", ZoneHints: sets.NewString("zone-b", "zone-c"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.5:80", ZoneHints: sets.NewString("zone-b", "zone-d"), Ready: true},
			&BaseEndpointInfo{Endpoint: "10.1.2.6:80", ZoneHints: sets.NewString("zone-c"), Ready: true},
		},
		clusterEndpoints: sets.NewString("10.1.2.3:80", "10.1.2.4:80", "10.1.2.6:80"),
		localEndpoints:   nil,
	}, {
		name:         "conflicting topology and localness require merging allEndpoints",
		hintsEnabled: true,
		nodeLabels:   map[string]string{v1.LabelTopologyZone: "zone-a"},
		serviceInfo:  &BaseServiceInfo{internalPolicyLocal: false, externalPolicyLocal: true, nodePort: 8080, hintsAnnotation: "auto"},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", ZoneHints: sets.NewString("zone-a"), Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", ZoneHints: sets.NewString("zone-b"), Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.2:80", ZoneHints: sets.NewString("zone-a"), Ready: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.3:80", ZoneHints: sets.NewString("zone-b"), Ready: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80", "10.0.0.2:80"),
		localEndpoints:   sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		allEndpoints:     sets.NewString("10.0.0.0:80", "10.0.0.1:80", "10.0.0.2:80"),
	}, {
		name:             "internalTrafficPolicy: Local, with empty endpoints",
		serviceInfo:      &BaseServiceInfo{internalPolicyLocal: true},
		endpoints:        []Endpoint{},
		clusterEndpoints: nil,
		localEndpoints:   sets.NewString(),
	}, {
		name:        "internalTrafficPolicy: Local, but all endpoints are remote",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: false},
		},
		clusterEndpoints:    nil,
		localEndpoints:      sets.NewString(),
		onlyRemoteEndpoints: true,
	}, {
		name:        "internalTrafficPolicy: Local, all endpoints are local",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: true},
		},
		clusterEndpoints: nil,
		localEndpoints:   sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
	}, {
		name:        "internalTrafficPolicy: Local, some endpoints are local",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: false},
		},
		clusterEndpoints: nil,
		localEndpoints:   sets.NewString("10.0.0.0:80"),
	}, {
		name:        "Cluster traffic policy, endpoints not Ready",
		serviceInfo: &BaseServiceInfo{},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: false},
		},
		clusterEndpoints: sets.NewString(),
		localEndpoints:   nil,
	}, {
		name:        "Cluster traffic policy, some endpoints are Ready",
		serviceInfo: &BaseServiceInfo{},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true},
		},
		clusterEndpoints: sets.NewString("10.0.0.1:80"),
		localEndpoints:   nil,
	}, {
		name:        "Cluster traffic policy, PTE enabled, all endpoints are terminating",
		pteEnabled:  true,
		serviceInfo: &BaseServiceInfo{},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: false, Serving: true, Terminating: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: false, Serving: true, Terminating: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		localEndpoints:   nil,
	}, {
		name:        "Cluster traffic policy, PTE disabled, all endpoints are terminating",
		pteEnabled:  false,
		serviceInfo: &BaseServiceInfo{},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: false, Serving: true, Terminating: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: false, Serving: true, Terminating: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString(),
		localEndpoints:   nil,
	}, {
		name:        "iTP: Local, eTP: Cluster, some endpoints local",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true, externalPolicyLocal: false, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		localEndpoints:   sets.NewString("10.0.0.0:80"),
		allEndpoints:     sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
	}, {
		name:        "iTP: Cluster, eTP: Local, some endpoints local",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: false, externalPolicyLocal: true, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		localEndpoints:   sets.NewString("10.0.0.0:80"),
		allEndpoints:     sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
	}, {
		name:        "iTP: Local, eTP: Local, some endpoints local",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true, externalPolicyLocal: true, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		localEndpoints:   sets.NewString("10.0.0.0:80"),
		allEndpoints:     sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
	}, {
		name:        "iTP: Local, eTP: Local, all endpoints remote",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true, externalPolicyLocal: true, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		localEndpoints:   sets.NewString(),
		allEndpoints:     sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
	}, {
		name:        "iTP: Local, eTP: Local, PTE disabled, all endpoints remote and terminating",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true, externalPolicyLocal: true, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: false, Serving: true, Terminating: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: false, Serving: true, Terminating: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString(),
		localEndpoints:   sets.NewString(),
		allEndpoints:     sets.NewString(),
	}, {
		name:        "iTP: Local, eTP: Local, PTE enabled, all endpoints remote and terminating",
		pteEnabled:  true,
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true, externalPolicyLocal: true, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: false, Serving: true, Terminating: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: false, Serving: true, Terminating: true, IsLocal: false},
		},
		clusterEndpoints:    sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		localEndpoints:      sets.NewString(),
		allEndpoints:        sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		onlyRemoteEndpoints: true,
	}, {
		name:        "iTP: Cluster, eTP: Local, PTE disabled, with terminating endpoints",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: false, externalPolicyLocal: true, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: false, Serving: false, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.2:80", Ready: false, Serving: true, Terminating: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.3:80", Ready: false, Serving: true, Terminating: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80"),
		localEndpoints:   sets.NewString(),
		allEndpoints:     sets.NewString("10.0.0.0:80"),
	}, {
		name:        "iTP: Cluster, eTP: Local, PTE enabled, with terminating endpoints",
		pteEnabled:  true,
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: false, externalPolicyLocal: true, nodePort: 8080},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: false, Serving: false, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.2:80", Ready: false, Serving: true, Terminating: true, IsLocal: true},
			&BaseEndpointInfo{Endpoint: "10.0.0.3:80", Ready: false, Serving: true, Terminating: true, IsLocal: false},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80"),
		localEndpoints:   sets.NewString("10.0.0.2:80"),
		allEndpoints:     sets.NewString("10.0.0.0:80", "10.0.0.2:80"),
	}, {
		name:        "externalTrafficPolicy ignored if not externally accessible",
		serviceInfo: &BaseServiceInfo{externalPolicyLocal: true},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: true},
		},
		clusterEndpoints: sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
		localEndpoints:   nil,
		allEndpoints:     sets.NewString("10.0.0.0:80", "10.0.0.1:80"),
	}, {
		name:        "no cluster endpoints for iTP:Local internal-only service",
		serviceInfo: &BaseServiceInfo{internalPolicyLocal: true},
		endpoints: []Endpoint{
			&BaseEndpointInfo{Endpoint: "10.0.0.0:80", Ready: true, IsLocal: false},
			&BaseEndpointInfo{Endpoint: "10.0.0.1:80", Ready: true, IsLocal: true},
		},
		clusterEndpoints: nil,
		localEndpoints:   sets.NewString("10.0.0.1:80"),
		allEndpoints:     sets.NewString("10.0.0.1:80"),
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.TopologyAwareHints, tc.hintsEnabled)()
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ProxyTerminatingEndpoints, tc.pteEnabled)()

			clusterEndpoints, localEndpoints, allEndpoints, hasAnyEndpoints := CategorizeEndpoints(tc.endpoints, tc.serviceInfo, tc.nodeLabels)

			if tc.clusterEndpoints == nil && clusterEndpoints != nil {
				t.Errorf("expected no cluster endpoints but got %v", clusterEndpoints)
			} else {
				err := checkExpectedEndpoints(tc.clusterEndpoints, clusterEndpoints)
				if err != nil {
					t.Errorf("error with cluster endpoints: %v", err)
				}
			}

			if tc.localEndpoints == nil && localEndpoints != nil {
				t.Errorf("expected no local endpoints but got %v", localEndpoints)
			} else {
				err := checkExpectedEndpoints(tc.localEndpoints, localEndpoints)
				if err != nil {
					t.Errorf("error with local endpoints: %v", err)
				}
			}

			var expectedAllEndpoints sets.String
			if tc.clusterEndpoints != nil && tc.localEndpoints == nil {
				expectedAllEndpoints = tc.clusterEndpoints
			} else if tc.localEndpoints != nil && tc.clusterEndpoints == nil {
				expectedAllEndpoints = tc.localEndpoints
			} else {
				expectedAllEndpoints = tc.allEndpoints
			}
			err := checkExpectedEndpoints(expectedAllEndpoints, allEndpoints)
			if err != nil {
				t.Errorf("error with allEndpoints: %v", err)
			}

			expectedHasAnyEndpoints := len(expectedAllEndpoints) > 0 || tc.onlyRemoteEndpoints
			if expectedHasAnyEndpoints != hasAnyEndpoints {
				t.Errorf("expected hasAnyEndpoints=%v, got %v", expectedHasAnyEndpoints, hasAnyEndpoints)
			}
		})
	}
}
