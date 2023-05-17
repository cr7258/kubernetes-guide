/*
Copyright 2020 The Kubernetes Authors.

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

package service

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	servicecontroller "k8s.io/cloud-provider/controllers/service"
	fakecloud "k8s.io/cloud-provider/fake"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/test/integration/framework"
	utilpointer "k8s.io/utils/pointer"
)

// Test_ServiceLoadBalancerAllocateNodePorts tests that a Service with spec.allocateLoadBalancerNodePorts=false
// does not allocate node ports for the Service.
func Test_ServiceLoadBalancerDisableAllocateNodePorts(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ServiceLBNodePortControl, true)()

	controlPlaneConfig := framework.NewIntegrationTestControlPlaneConfig()
	_, server, closeFn := framework.RunAnAPIServer(controlPlaneConfig)
	defer closeFn()

	config := restclient.Config{Host: server.URL}
	client, err := clientset.NewForConfig(&config)
	if err != nil {
		t.Fatalf("Error creating clientset: %v", err)
	}

	ns := framework.CreateTestingNamespace("test-service-allocate-node-ports", server, t)
	defer framework.DeleteTestingNamespace(ns, server, t)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-123",
		},
		Spec: corev1.ServiceSpec{
			Type:                          corev1.ServiceTypeLoadBalancer,
			AllocateLoadBalancerNodePorts: utilpointer.BoolPtr(false),
			Ports: []corev1.ServicePort{{
				Port: int32(80),
			}},
			Selector: map[string]string{
				"foo": "bar",
			},
		},
	}

	service, err = client.CoreV1().Services(ns.Name).Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error creating test service: %v", err)
	}

	if serviceHasNodePorts(service) {
		t.Error("found node ports when none was expected")
	}
}

// Test_ServiceLoadBalancerSwitchToDeallocatedNodePorts test that switching a Service
// to spec.allocateLoadBalancerNodePorts=false, does not de-allocate existing node ports.
func Test_ServiceLoadBalancerEnableThenDisableAllocatedNodePorts(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ServiceLBNodePortControl, true)()

	controlPlaneConfig := framework.NewIntegrationTestControlPlaneConfig()
	_, server, closeFn := framework.RunAnAPIServer(controlPlaneConfig)
	defer closeFn()

	config := restclient.Config{Host: server.URL}
	client, err := clientset.NewForConfig(&config)
	if err != nil {
		t.Fatalf("Error creating clientset: %v", err)
	}

	ns := framework.CreateTestingNamespace("test-service-deallocate-node-ports", server, t)
	defer framework.DeleteTestingNamespace(ns, server, t)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-123",
		},
		Spec: corev1.ServiceSpec{
			Type:                          corev1.ServiceTypeLoadBalancer,
			AllocateLoadBalancerNodePorts: utilpointer.BoolPtr(true),
			Ports: []corev1.ServicePort{{
				Port: int32(80),
			}},
			Selector: map[string]string{
				"foo": "bar",
			},
		},
	}

	service, err = client.CoreV1().Services(ns.Name).Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error creating test service: %v", err)
	}

	if !serviceHasNodePorts(service) {
		t.Error("expected node ports but found none")
	}

	service.Spec.AllocateLoadBalancerNodePorts = utilpointer.BoolPtr(false)
	service, err = client.CoreV1().Services(ns.Name).Update(context.TODO(), service, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Error updating test service: %v", err)
	}

	if !serviceHasNodePorts(service) {
		t.Error("node ports were unexpectedly deallocated")
	}
}

func serviceHasNodePorts(svc *corev1.Service) bool {
	for _, port := range svc.Spec.Ports {
		if port.NodePort > 0 {
			return true
		}
	}

	return false
}

// Test_ServiceLoadBalancerEnableLoadBalancerClass tests that when a LoadBalancer
// type of service has spec.LoadBalancerClass set, cloud provider should not create default load balancer.
func Test_ServiceLoadBalancerEnableLoadBalancerClass(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ServiceLoadBalancerClass, true)()

	controlPlaneConfig := framework.NewIntegrationTestControlPlaneConfig()
	_, server, closeFn := framework.RunAnAPIServer(controlPlaneConfig)
	defer closeFn()

	config := restclient.Config{Host: server.URL}
	client, err := clientset.NewForConfig(&config)
	if err != nil {
		t.Fatalf("Error creating clientset: %v", err)
	}

	ns := framework.CreateTestingNamespace("test-service-load-balancer-class", server, t)
	defer framework.DeleteTestingNamespace(ns, server, t)

	controller, cloud, informer := newServiceController(t, client)

	stopCh := make(chan struct{})
	informer.Start(stopCh)
	go controller.Run(stopCh, 1)
	defer close(stopCh)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-load-balancer-class",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{{
				Port: int32(80),
			}},
			LoadBalancerClass: utilpointer.StringPtr("test.com/test"),
		},
	}

	_, err = client.CoreV1().Services(ns.Name).Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error creating test service: %v", err)
	}

	if len(cloud.Calls) > 0 {
		t.Errorf("Unexpected cloud provider calls: %v", cloud.Calls)
	}
}

// Test_ServiceLoadBalancerEnableLoadBalancerClassThenUpdateLoadBalancerClass tests that when a LoadBalancer
// type of service has spec.LoadBalancerClass set, it should be immutable as long as the service type
// is still LoadBalancer.
func Test_ServiceLoadBalancerEnableLoadBalancerClassThenUpdateLoadBalancerClass(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ServiceLoadBalancerClass, true)()

	controlPlaneConfig := framework.NewIntegrationTestControlPlaneConfig()
	_, server, closeFn := framework.RunAnAPIServer(controlPlaneConfig)
	defer closeFn()

	config := restclient.Config{Host: server.URL}
	client, err := clientset.NewForConfig(&config)
	if err != nil {
		t.Fatalf("Error creating clientset: %v", err)
	}

	ns := framework.CreateTestingNamespace("test-service-immutable-load-balancer-class", server, t)
	defer framework.DeleteTestingNamespace(ns, server, t)

	controller, cloud, informer := newServiceController(t, client)

	stopCh := make(chan struct{})
	informer.Start(stopCh)
	go controller.Run(stopCh, 1)
	defer close(stopCh)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-load-balancer-class",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{{
				Port: int32(80),
			}},
			LoadBalancerClass: utilpointer.StringPtr("test.com/test"),
		},
	}

	service, err = client.CoreV1().Services(ns.Name).Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error creating test service: %v", err)
	}

	if len(cloud.Calls) > 0 {
		t.Errorf("Unexpected cloud provider calls: %v", cloud.Calls)
	}

	service.Spec.LoadBalancerClass = utilpointer.StringPtr("test.com/update")
	_, err = client.CoreV1().Services(ns.Name).Update(context.TODO(), service, metav1.UpdateOptions{})
	if err == nil {
		t.Fatal("Error updating test service load balancer class should throw error")
	}

	if len(cloud.Calls) > 0 {
		t.Errorf("Unexpected cloud provider calls: %v", cloud.Calls)
	}
}

func newServiceController(t *testing.T, client *clientset.Clientset) (*servicecontroller.Controller, *fakecloud.Cloud, informers.SharedInformerFactory) {
	cloud := &fakecloud.Cloud{}
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	serviceInformer := informerFactory.Core().V1().Services()
	nodeInformer := informerFactory.Core().V1().Nodes()

	controller, err := servicecontroller.New(cloud,
		client,
		serviceInformer,
		nodeInformer,
		"test-cluster",
		utilfeature.DefaultFeatureGate)
	if err != nil {
		t.Fatalf("Error creating service controller: %v", err)
	}
	cloud.Calls = nil // ignore any cloud calls made in init()
	return controller, cloud, informerFactory
}
