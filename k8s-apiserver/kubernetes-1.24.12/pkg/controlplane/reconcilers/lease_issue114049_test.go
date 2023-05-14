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

package reconcilers

/*
This is the same test added in
https://github.com/kubernetes/kubernetes/commit/4ffca653ff71525852d1a8abc5923156b22f9ced
to cover the regression in https://issues.k8s.io/114049, but split into a different file
to be able to backport it without having to backport 7 additional commits that pull non-test
changes.
*/

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/apitesting"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/storage"
	etcd3testing "k8s.io/apiserver/pkg/storage/etcd3/testing"
	"k8s.io/apiserver/pkg/storage/storagebackend/factory"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/apis/core"
	netutils "k8s.io/utils/net"
)

func init() {
	var scheme = runtime.NewScheme()

	metav1.AddToGroupVersion(scheme, metav1.SchemeGroupVersion)
	utilruntime.Must(core.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(scheme.SetVersionPriority(corev1.SchemeGroupVersion))

	codecs = serializer.NewCodecFactory(scheme)
}

var codecs serializer.CodecFactory

type fakeLeasesIssue114049 struct {
	storageLeases
}

var _ Leases = &fakeLeasesIssue114049{}

func newFakeLeasesIssue114049(t *testing.T, s storage.Interface) *fakeLeasesIssue114049 {
	// use the same base key used by the controlplane, but add a random
	// prefix so we can reuse the etcd instance for subtests independently.
	// pkg/controlplane/instance.go:268:
	// masterLeases, err := reconcilers.NewLeases(config, "/masterleases/", ttl)
	// ref: https://issues.k8s.io/114049
	base := "/" + uuid.New().String() + "/masterleases/"
	return &fakeLeasesIssue114049{
		storageLeases{
			storage:   s,
			baseKey:   base,
			leaseTime: 1 * time.Minute, // avoid the lease to timeout on tests
		},
	}
}

func (f *fakeLeasesIssue114049) SetKeys(keys []string) error {
	for _, ip := range keys {
		if err := f.UpdateLease(ip); err != nil {
			return err
		}
	}
	return nil
}

func TestLeaseEndpointReconcilerIssue114049(t *testing.T) {
	server, sc := etcd3testing.NewUnsecuredEtcd3TestClientServer(t)
	t.Cleanup(func() { server.Terminate(t) })

	newFunc := func() runtime.Object { return &corev1.Endpoints{} }
	sc.Codec = apitesting.TestStorageCodec(codecs, corev1.SchemeGroupVersion)

	s, dFunc, err := factory.Create(*sc.ForResource(schema.GroupResource{Resource: "endpoints"}), newFunc)
	if err != nil {
		t.Fatalf("Error creating storage: %v", err)
	}
	t.Cleanup(dFunc)

	reconcileTests := []struct {
		testName      string
		serviceName   string
		ip            string
		endpointPorts []corev1.EndpointPort
		endpointKeys  []string
		initialState  []runtime.Object
		expectUpdate  []runtime.Object
		expectCreate  []runtime.Object
		expectLeases  []string
	}{
		{
			testName:      "no existing endpoints",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState:  nil,
			expectCreate:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints satisfy",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints satisfy, no endpointslice",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState: []runtime.Object{
				makeEndpoints("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectCreate: []runtime.Object{
				makeEndpointSlice("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectLeases: []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpointslice satisfies, no endpoints",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState: []runtime.Object{
				makeEndpointSlice("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectCreate: []runtime.Object{
				makeEndpoints("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectLeases: []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints satisfy, endpointslice is wrong",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState: []runtime.Object{
				makeEndpoints("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
				makeEndpointSlice("foo", []string{"4.3.2.1"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectUpdate: []runtime.Object{
				makeEndpointSlice("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectLeases: []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpointslice satisfies, endpoints is wrong",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState: []runtime.Object{
				makeEndpoints("foo", []string{"4.3.2.1"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
				makeEndpointSlice("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectUpdate: []runtime.Object{
				makeEndpoints("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectLeases: []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints satisfy + refresh existing key",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			endpointKeys:  []string{"1.2.3.4"},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints satisfy but too many",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4", "4.3.2.1"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints satisfy but too many + extra masters",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			endpointKeys:  []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4", "4.3.2.1", "4.3.2.2", "4.3.2.3", "4.3.2.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"},
		},
		{
			testName:      "existing endpoints satisfy but too many + extra masters + delete first",
			serviceName:   "foo",
			ip:            "4.3.2.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			endpointKeys:  []string{"4.3.2.1", "4.3.2.2", "4.3.2.3", "4.3.2.4"},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4", "4.3.2.1", "4.3.2.2", "4.3.2.3", "4.3.2.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"4.3.2.1", "4.3.2.2", "4.3.2.3", "4.3.2.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"4.3.2.1", "4.3.2.2", "4.3.2.3", "4.3.2.4"},
		},
		{
			testName:      "existing endpoints current IP missing",
			serviceName:   "foo",
			ip:            "4.3.2.2",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			endpointKeys:  []string{"4.3.2.1"},
			initialState:  makeEndpointsArray("foo", []string{"4.3.2.1"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"4.3.2.1", "4.3.2.2"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"4.3.2.1", "4.3.2.2"},
		},
		{
			testName:      "existing endpoints wrong name",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState:  makeEndpointsArray("bar", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectCreate:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints wrong IP",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState:  makeEndpointsArray("foo", []string{"4.3.2.1"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints wrong port",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 9090, Protocol: "TCP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints wrong protocol",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "UDP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints wrong port name",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "baz", Port: 8080, Protocol: "TCP"}},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "baz", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
		{
			testName:      "existing endpoints without skip mirror label",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState: []runtime.Object{
				// can't use makeEndpointsArray() here because we don't want the
				// skip-mirror label
				&corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: metav1.NamespaceDefault,
						Name:      "foo",
					},
					Subsets: []corev1.EndpointSubset{{
						Addresses: []corev1.EndpointAddress{{IP: "1.2.3.4"}},
						Ports:     []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
					}},
				},
				makeEndpointSlice("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			},
			expectUpdate: []runtime.Object{
				makeEndpoints("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
				// EndpointSlice does not get updated because it was already correct
			},
			expectLeases: []string{"1.2.3.4"},
		},
		{
			testName:    "existing endpoints extra service ports satisfy",
			serviceName: "foo",
			ip:          "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{
				{Name: "foo", Port: 8080, Protocol: "TCP"},
				{Name: "bar", Port: 1000, Protocol: "TCP"},
				{Name: "baz", Port: 1010, Protocol: "TCP"},
			},
			initialState: makeEndpointsArray("foo", []string{"1.2.3.4"},
				[]corev1.EndpointPort{
					{Name: "foo", Port: 8080, Protocol: "TCP"},
					{Name: "bar", Port: 1000, Protocol: "TCP"},
					{Name: "baz", Port: 1010, Protocol: "TCP"},
				},
			),
			expectLeases: []string{"1.2.3.4"},
		},
		{
			testName:    "existing endpoints extra service ports missing port",
			serviceName: "foo",
			ip:          "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{
				{Name: "foo", Port: 8080, Protocol: "TCP"},
				{Name: "bar", Port: 1000, Protocol: "TCP"},
			},
			initialState: makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate: makeEndpointsArray("foo", []string{"1.2.3.4"},
				[]corev1.EndpointPort{
					{Name: "foo", Port: 8080, Protocol: "TCP"},
					{Name: "bar", Port: 1000, Protocol: "TCP"},
				},
			),
			expectLeases: []string{"1.2.3.4"},
		},
	}
	for _, test := range reconcileTests {
		t.Run(test.testName, func(t *testing.T) {
			fakeLeases := newFakeLeasesIssue114049(t, s)
			err := fakeLeases.SetKeys(test.endpointKeys)
			if err != nil {
				t.Errorf("unexpected error creating keys: %v", err)
			}
			clientset := fake.NewSimpleClientset(test.initialState...)

			epAdapter := NewEndpointsAdapter(clientset.CoreV1(), clientset.DiscoveryV1())
			r := NewLeaseEndpointReconciler(epAdapter, fakeLeases)
			err = r.ReconcileEndpoints(test.serviceName, netutils.ParseIPSloppy(test.ip), test.endpointPorts, true)
			if err != nil {
				t.Errorf("unexpected error reconciling: %v", err)
			}

			err = verifyCreatesAndUpdates(clientset, test.expectCreate, test.expectUpdate)
			if err != nil {
				t.Errorf("unexpected error in side effects: %v", err)
			}

			leases, err := fakeLeases.ListLeases()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			// sort for comparison
			sort.Strings(leases)
			sort.Strings(test.expectLeases)
			if !reflect.DeepEqual(leases, test.expectLeases) {
				t.Errorf("expected %v got: %v", test.expectLeases, leases)
			}
		})
	}

	nonReconcileTests := []struct {
		testName      string
		serviceName   string
		ip            string
		endpointPorts []corev1.EndpointPort
		endpointKeys  []string
		initialState  []runtime.Object
		expectUpdate  []runtime.Object
		expectCreate  []runtime.Object
		expectLeases  []string
	}{
		{
			testName:    "existing endpoints extra service ports missing port no update",
			serviceName: "foo",
			ip:          "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{
				{Name: "foo", Port: 8080, Protocol: "TCP"},
				{Name: "bar", Port: 1000, Protocol: "TCP"},
			},
			initialState: makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate: nil,
			expectLeases: []string{"1.2.3.4"},
		},
		{
			testName:    "existing endpoints extra service ports, wrong ports, wrong IP",
			serviceName: "foo",
			ip:          "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{
				{Name: "foo", Port: 8080, Protocol: "TCP"},
				{Name: "bar", Port: 1000, Protocol: "TCP"},
			},
			initialState: makeEndpointsArray("foo", []string{"4.3.2.1"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate: makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases: []string{"1.2.3.4"},
		},
		{
			testName:      "no existing endpoints",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			initialState:  nil,
			expectCreate:  makeEndpointsArray("foo", []string{"1.2.3.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4"},
		},
	}
	for _, test := range nonReconcileTests {
		t.Run(test.testName, func(t *testing.T) {
			fakeLeases := newFakeLeasesIssue114049(t, s)
			err := fakeLeases.SetKeys(test.endpointKeys)
			if err != nil {
				t.Errorf("unexpected error creating keys: %v", err)
			}
			clientset := fake.NewSimpleClientset(test.initialState...)
			epAdapter := NewEndpointsAdapter(clientset.CoreV1(), clientset.DiscoveryV1())
			r := NewLeaseEndpointReconciler(epAdapter, fakeLeases)
			err = r.ReconcileEndpoints(test.serviceName, netutils.ParseIPSloppy(test.ip), test.endpointPorts, false)
			if err != nil {
				t.Errorf("unexpected error reconciling: %v", err)
			}

			err = verifyCreatesAndUpdates(clientset, test.expectCreate, test.expectUpdate)
			if err != nil {
				t.Errorf("unexpected error in side effects: %v", err)
			}

			leases, err := fakeLeases.ListLeases()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			// sort for comparison
			sort.Strings(leases)
			sort.Strings(test.expectLeases)
			if !reflect.DeepEqual(leases, test.expectLeases) {
				t.Errorf("expected %v got: %v", test.expectLeases, leases)
			}
		})
	}
}

func TestLeaseRemoveEndpointsIssue114049(t *testing.T) {
	server, sc := etcd3testing.NewUnsecuredEtcd3TestClientServer(t)
	t.Cleanup(func() { server.Terminate(t) })

	newFunc := func() runtime.Object { return &corev1.Endpoints{} }
	sc.Codec = apitesting.TestStorageCodec(codecs, corev1.SchemeGroupVersion)

	s, dFunc, err := factory.Create(*sc.ForResource(schema.GroupResource{Resource: "pods"}), newFunc)
	if err != nil {
		t.Fatalf("Error creating storage: %v", err)
	}
	t.Cleanup(dFunc)

	stopTests := []struct {
		testName      string
		serviceName   string
		ip            string
		endpointPorts []corev1.EndpointPort
		endpointKeys  []string
		initialState  []runtime.Object
		expectUpdate  []runtime.Object
		expectLeases  []string
	}{
		{
			testName:      "successful stop reconciling",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			endpointKeys:  []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectUpdate:  makeEndpointsArray("foo", []string{"4.3.2.2", "4.3.2.3", "4.3.2.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"4.3.2.2", "4.3.2.3", "4.3.2.4"},
		},
		{
			testName:      "stop reconciling with ip not in endpoint ip list",
			serviceName:   "foo",
			ip:            "5.6.7.8",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			endpointKeys:  []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"},
			initialState:  makeEndpointsArray("foo", []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"},
		},
		{
			testName:      "endpoint with no subset",
			serviceName:   "foo",
			ip:            "1.2.3.4",
			endpointPorts: []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}},
			endpointKeys:  []string{"1.2.3.4", "4.3.2.2", "4.3.2.3", "4.3.2.4"},
			initialState:  makeEndpointsArray("foo", nil, nil),
			expectUpdate:  makeEndpointsArray("foo", []string{"4.3.2.2", "4.3.2.3", "4.3.2.4"}, []corev1.EndpointPort{{Name: "foo", Port: 8080, Protocol: "TCP"}}),
			expectLeases:  []string{"4.3.2.2", "4.3.2.3", "4.3.2.4"},
		},
	}
	for _, test := range stopTests {
		t.Run(test.testName, func(t *testing.T) {
			fakeLeases := newFakeLeasesIssue114049(t, s)
			err := fakeLeases.SetKeys(test.endpointKeys)
			if err != nil {
				t.Errorf("unexpected error creating keys: %v", err)
			}
			clientset := fake.NewSimpleClientset(test.initialState...)
			epAdapter := NewEndpointsAdapter(clientset.CoreV1(), clientset.DiscoveryV1())
			r := NewLeaseEndpointReconciler(epAdapter, fakeLeases)
			err = r.RemoveEndpoints(test.serviceName, netutils.ParseIPSloppy(test.ip), test.endpointPorts)
			// if the ip is not on the endpoints, it must return an storage error and stop reconciling
			if !contains(test.endpointKeys, test.ip) {
				if !storage.IsNotFound(err) {
					t.Errorf("expected error StorageError: key not found, Code: 1, Key: /registry/base/key/%s got:  %v", test.ip, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error reconciling: %v", err)
			}

			err = verifyCreatesAndUpdates(clientset, nil, test.expectUpdate)
			if err != nil {
				t.Errorf("unexpected error in side effects: %v", err)
			}

			leases, err := fakeLeases.ListLeases()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			// sort for comparison
			sort.Strings(leases)
			sort.Strings(test.expectLeases)
			if !reflect.DeepEqual(leases, test.expectLeases) {
				t.Errorf("expected %v got: %v", test.expectLeases, leases)
			}
		})
	}
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}
