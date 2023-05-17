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

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/network/common"
	imageutils "k8s.io/kubernetes/test/utils/image"

	"github.com/onsi/ginkgo"
)

var _ = common.SIGDescribe("EndpointSlice", func() {
	f := framework.NewDefaultFramework("endpointslice")

	var cs clientset.Interface
	var podClient *framework.PodClient

	ginkgo.BeforeEach(func() {
		cs = f.ClientSet
		podClient = f.PodClient()
	})

	/*
		Release: v1.21
		Testname: EndpointSlice API
		Description: The discovery.k8s.io API group MUST exist in the /apis discovery document.
		The discovery.k8s.io/v1 API group/version MUST exist in the /apis/discovery.k8s.io discovery document.
		The endpointslices resource MUST exist in the /apis/discovery.k8s.io/v1 discovery document.
		API Server should create self referential Endpoints and EndpointSlices named "kubernetes" in the default namespace.
	*/
	framework.ConformanceIt("should have Endpoints and EndpointSlices pointing to API Server", func() {
		namespace := "default"
		name := "kubernetes"
		endpoints, err := cs.CoreV1().Endpoints(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		framework.ExpectNoError(err, "error creating Endpoints resource")
		if len(endpoints.Subsets) != 1 {
			framework.Failf("Expected 1 subset in endpoints, got %d: %#v", len(endpoints.Subsets), endpoints.Subsets)
		}

		endpointSubset := endpoints.Subsets[0]
		endpointSlice, err := cs.DiscoveryV1().EndpointSlices(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		framework.ExpectNoError(err, "error creating EndpointSlice resource")
		if len(endpointSlice.Ports) != len(endpointSubset.Ports) {
			framework.Failf("Expected EndpointSlice to have %d ports, got %d: %#v", len(endpointSubset.Ports), len(endpointSlice.Ports), endpointSlice.Ports)
		}
		numExpectedEndpoints := len(endpointSubset.Addresses) + len(endpointSubset.NotReadyAddresses)
		if len(endpointSlice.Endpoints) != numExpectedEndpoints {
			framework.Failf("Expected EndpointSlice to have %d endpoints, got %d: %#v", numExpectedEndpoints, len(endpointSlice.Endpoints), endpointSlice.Endpoints)
		}

	})

	/*
		Release: v1.21
		Testname: EndpointSlice API
		Description: The discovery.k8s.io API group MUST exist in the /apis discovery document.
		The discovery.k8s.io/v1 API group/version MUST exist in the /apis/discovery.k8s.io discovery document.
		The endpointslices resource MUST exist in the /apis/discovery.k8s.io/v1 discovery document.
		The endpointslice controller should create and delete EndpointSlices for Pods matching a Service.
	*/
	framework.ConformanceIt("should create and delete Endpoints and EndpointSlices for a Service with a selector specified", func() {
		svc := createServiceReportErr(cs, f.Namespace.Name, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example-empty-selector",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"does-not-match-anything": "endpoints-and-endpoint-slices-should-still-be-created",
				},
				Ports: []v1.ServicePort{{
					Name:     "example",
					Port:     80,
					Protocol: v1.ProtocolTCP,
				}},
			},
		})

		// Expect Endpoints resource to be created.
		if err := wait.PollImmediate(2*time.Second, wait.ForeverTestTimeout, func() (bool, error) {
			_, err := cs.CoreV1().Endpoints(svc.Namespace).Get(context.TODO(), svc.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			return true, nil
		}); err != nil {
			framework.Failf("No Endpoints found for Service %s/%s: %s", svc.Namespace, svc.Name, err)
		}

		// Expect EndpointSlice resource to be created.
		var endpointSlice discoveryv1.EndpointSlice
		if err := wait.PollImmediate(2*time.Second, wait.ForeverTestTimeout, func() (bool, error) {
			endpointSliceList, err := cs.DiscoveryV1().EndpointSlices(svc.Namespace).List(context.TODO(), metav1.ListOptions{
				LabelSelector: "kubernetes.io/service-name=" + svc.Name,
			})
			if err != nil {
				return false, err
			}
			if len(endpointSliceList.Items) == 0 {
				return false, nil
			}
			endpointSlice = endpointSliceList.Items[0]
			return true, nil
		}); err != nil {
			framework.Failf("No EndpointSlice found for Service %s/%s: %s", svc.Namespace, svc.Name, err)
		}

		// Ensure EndpointSlice has expected values.
		managedBy, ok := endpointSlice.Labels[discoveryv1.LabelManagedBy]
		expectedManagedBy := "endpointslice-controller.k8s.io"
		if !ok {
			framework.Failf("Expected EndpointSlice to have %s label, got %#v", discoveryv1.LabelManagedBy, endpointSlice.Labels)
		} else if managedBy != expectedManagedBy {
			framework.Failf("Expected EndpointSlice to have %s label with %s value, got %s", discoveryv1.LabelManagedBy, expectedManagedBy, managedBy)
		}
		if len(endpointSlice.Endpoints) != 0 {
			framework.Failf("Expected EndpointSlice to have 0 endpoints, got %d: %#v", len(endpointSlice.Endpoints), endpointSlice.Endpoints)
		}

		err := cs.CoreV1().Services(svc.Namespace).Delete(context.TODO(), svc.Name, metav1.DeleteOptions{})
		framework.ExpectNoError(err, "error deleting Service")

		// Expect Endpoints resource to be deleted when Service is.
		if err := wait.PollImmediate(2*time.Second, wait.ForeverTestTimeout, func() (bool, error) {
			_, err := cs.CoreV1().Endpoints(svc.Namespace).Get(context.TODO(), svc.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return false, nil
		}); err != nil {
			framework.Failf("Endpoints resource not deleted after Service %s/%s was deleted: %s", svc.Namespace, svc.Name, err)
		}

		// Expect EndpointSlice resource to be deleted when Service is. Wait for
		// up to 90 seconds since garbage collector only polls every 30 seconds
		// and may need to retry informer resync at some point during an e2e
		// run.
		if err := wait.PollImmediate(2*time.Second, 90*time.Second, func() (bool, error) {
			endpointSliceList, err := cs.DiscoveryV1().EndpointSlices(svc.Namespace).List(context.TODO(), metav1.ListOptions{
				LabelSelector: "kubernetes.io/service-name=" + svc.Name,
			})
			if err != nil {
				return false, err
			}
			if len(endpointSliceList.Items) == 0 {
				return true, nil
			}
			return false, nil
		}); err != nil {
			framework.Failf("EndpointSlice resource not deleted after Service %s/%s was deleted: %s", svc.Namespace, svc.Name, err)
		}
	})

	/*
		Release: v1.21
		Testname: EndpointSlice API
		Description: The discovery.k8s.io API group MUST exist in the /apis discovery document.
		The discovery.k8s.io/v1 API group/version MUST exist in the /apis/discovery.k8s.io discovery document.
		The endpointslices resource MUST exist in the /apis/discovery.k8s.io/v1 discovery document.
		The endpointslice controller must create EndpointSlices for Pods mataching a Service.
	*/
	framework.ConformanceIt("should create Endpoints and EndpointSlices for Pods matching a Service", func() {
		labelPod1 := "pod1"
		labelPod2 := "pod2"
		labelPod3 := "pod3"
		labelShared12 := "shared12"
		labelValue := "on"

		pod1 := podClient.Create(&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod1",
				Labels: map[string]string{
					labelPod1:     labelValue,
					labelShared12: labelValue,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "container1",
						Image: imageutils.GetE2EImage(imageutils.Nginx),
						Ports: []v1.ContainerPort{{
							Name:          "example-name",
							ContainerPort: int32(3000),
						}},
					},
				},
			},
		})

		pod2 := podClient.Create(&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod2",
				Labels: map[string]string{
					labelPod2:     labelValue,
					labelShared12: labelValue,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "container1",
						Image: imageutils.GetE2EImage(imageutils.Nginx),
						Ports: []v1.ContainerPort{{
							Name:          "example-name",
							ContainerPort: int32(3001),
						}, {
							Name:          "other-port",
							ContainerPort: int32(3002),
						}},
					},
				},
			},
		})

		svc1 := createServiceReportErr(cs, f.Namespace.Name, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example-int-port",
			},
			Spec: v1.ServiceSpec{
				Selector:                 map[string]string{labelPod1: labelValue},
				PublishNotReadyAddresses: true,
				Ports: []v1.ServicePort{{
					Name:       "example",
					Port:       80,
					TargetPort: intstr.FromInt(3000),
					Protocol:   v1.ProtocolTCP,
				}},
			},
		})

		svc2 := createServiceReportErr(cs, f.Namespace.Name, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example-named-port",
			},
			Spec: v1.ServiceSpec{
				Selector:                 map[string]string{labelShared12: labelValue},
				PublishNotReadyAddresses: true,
				Ports: []v1.ServicePort{{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("example-name"),
					Protocol:   v1.ProtocolTCP,
				}},
			},
		})

		svc3 := createServiceReportErr(cs, f.Namespace.Name, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example-no-match",
			},
			Spec: v1.ServiceSpec{
				Selector:                 map[string]string{labelPod3: labelValue},
				PublishNotReadyAddresses: true,
				Ports: []v1.ServicePort{{
					Name:       "example-no-match",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   v1.ProtocolTCP,
				}},
			},
		})

		err := wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
			var err error
			pod1, err = podClient.Get(context.TODO(), pod1.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if len(pod1.Status.PodIPs) == 0 {
				return false, nil
			}

			pod2, err = podClient.Get(context.TODO(), pod2.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if len(pod2.Status.PodIPs) == 0 {
				return false, nil
			}

			return true, nil
		})
		framework.ExpectNoError(err, "timed out waiting for Pods to have IPs assigned")

		ginkgo.By("referencing a single matching pod")
		expectEndpointsAndSlices(cs, f.Namespace.Name, svc1, []*v1.Pod{pod1}, 1, 1, false)

		ginkgo.By("referencing matching pods with named port")
		expectEndpointsAndSlices(cs, f.Namespace.Name, svc2, []*v1.Pod{pod1, pod2}, 2, 2, true)

		ginkgo.By("creating empty Endpoints and EndpointSlices for no matching Pods")
		expectEndpointsAndSlices(cs, f.Namespace.Name, svc3, []*v1.Pod{}, 0, 1, false)

		// TODO: Update test to cover Endpoints recreation after deletes once it
		// actually works.
		ginkgo.By("recreating EndpointSlices after they've been deleted")
		deleteEndpointSlices(cs, f.Namespace.Name, svc2)
		expectEndpointsAndSlices(cs, f.Namespace.Name, svc2, []*v1.Pod{pod1, pod2}, 2, 2, true)
	})

	/*
		Release: v1.21
		Testname: EndpointSlice API
		Description: The discovery.k8s.io API group MUST exist in the /apis discovery document.
		The discovery.k8s.io/v1 API group/version MUST exist in the /apis/discovery.k8s.io discovery document.
		The endpointslices resource MUST exist in the /apis/discovery.k8s.io/v1 discovery document.
		The endpointslices resource must support create, get, list, watch, update, patch, delete, and deletecollection.
	*/
	framework.ConformanceIt("should support creating EndpointSlice API operations", func() {
		// Setup
		ns := f.Namespace.Name
		epsVersion := "v1"
		epsClient := f.ClientSet.DiscoveryV1().EndpointSlices(ns)

		epsTemplate := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "e2e-example-ing",
				Labels: map[string]string{
					"special-label": f.UniqueName,
				}},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"1.2.3.4", "5.6.7.8"}},
				{Addresses: []string{"2.2.3.4", "6.6.7.8"}},
			},
		}
		// Discovery
		ginkgo.By("getting /apis")
		{
			discoveryGroups, err := f.ClientSet.Discovery().ServerGroups()
			framework.ExpectNoError(err)
			found := false
			for _, group := range discoveryGroups.Groups {
				if group.Name == discoveryv1.GroupName {
					for _, version := range group.Versions {
						if version.Version == epsVersion {
							found = true
							break
						}
					}
				}
			}
			framework.ExpectEqual(found, true, fmt.Sprintf("expected discovery API group/version, got %#v", discoveryGroups.Groups))
		}

		ginkgo.By("getting /apis/discovery.k8s.io")
		{
			group := &metav1.APIGroup{}
			err := f.ClientSet.Discovery().RESTClient().Get().AbsPath("/apis/discovery.k8s.io").Do(context.TODO()).Into(group)
			framework.ExpectNoError(err)
			found := false
			for _, version := range group.Versions {
				if version.Version == epsVersion {
					found = true
					break
				}
			}
			framework.ExpectEqual(found, true, fmt.Sprintf("expected discovery API version, got %#v", group.Versions))
		}

		ginkgo.By("getting /apis/discovery.k8s.io" + epsVersion)
		{
			resources, err := f.ClientSet.Discovery().ServerResourcesForGroupVersion(discoveryv1.SchemeGroupVersion.String())
			framework.ExpectNoError(err)
			foundEPS := false
			for _, resource := range resources.APIResources {
				switch resource.Name {
				case "endpointslices":
					foundEPS = true
				}
			}
			framework.ExpectEqual(foundEPS, true, fmt.Sprintf("expected endpointslices, got %#v", resources.APIResources))
		}

		// EndpointSlice resource create/read/update/watch verbs
		ginkgo.By("creating")
		_, err := epsClient.Create(context.TODO(), epsTemplate, metav1.CreateOptions{})
		framework.ExpectNoError(err)
		_, err = epsClient.Create(context.TODO(), epsTemplate, metav1.CreateOptions{})
		framework.ExpectNoError(err)
		createdEPS, err := epsClient.Create(context.TODO(), epsTemplate, metav1.CreateOptions{})
		framework.ExpectNoError(err)

		ginkgo.By("getting")
		queriedEPS, err := epsClient.Get(context.TODO(), createdEPS.Name, metav1.GetOptions{})
		framework.ExpectNoError(err)
		framework.ExpectEqual(queriedEPS.UID, createdEPS.UID)

		ginkgo.By("listing")
		epsList, err := epsClient.List(context.TODO(), metav1.ListOptions{LabelSelector: "special-label=" + f.UniqueName})
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(epsList.Items), 3, "filtered list should have 3 items")

		ginkgo.By("watching")
		framework.Logf("starting watch")
		epsWatch, err := epsClient.Watch(context.TODO(), metav1.ListOptions{ResourceVersion: epsList.ResourceVersion, LabelSelector: "special-label=" + f.UniqueName})
		framework.ExpectNoError(err)

		// Test cluster-wide list and watch
		clusterEPSClient := f.ClientSet.DiscoveryV1().EndpointSlices("")
		ginkgo.By("cluster-wide listing")
		clusterEPSList, err := clusterEPSClient.List(context.TODO(), metav1.ListOptions{LabelSelector: "special-label=" + f.UniqueName})
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(clusterEPSList.Items), 3, "filtered list should have 3 items")

		ginkgo.By("cluster-wide watching")
		framework.Logf("starting watch")
		_, err = clusterEPSClient.Watch(context.TODO(), metav1.ListOptions{ResourceVersion: epsList.ResourceVersion, LabelSelector: "special-label=" + f.UniqueName})
		framework.ExpectNoError(err)

		ginkgo.By("patching")
		patchedEPS, err := epsClient.Patch(context.TODO(), createdEPS.Name, types.MergePatchType, []byte(`{"metadata":{"annotations":{"patched":"true"}}}`), metav1.PatchOptions{})
		framework.ExpectNoError(err)
		framework.ExpectEqual(patchedEPS.Annotations["patched"], "true", "patched object should have the applied annotation")

		ginkgo.By("updating")
		var epsToUpdate, updatedEPS *discoveryv1.EndpointSlice
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			epsToUpdate, err = epsClient.Get(context.TODO(), createdEPS.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			epsToUpdate.Annotations["updated"] = "true"
			updatedEPS, err = epsClient.Update(context.TODO(), epsToUpdate, metav1.UpdateOptions{})
			return err
		})
		framework.ExpectNoError(err)
		framework.ExpectEqual(updatedEPS.Annotations["updated"], "true", "updated object should have the applied annotation")

		framework.Logf("waiting for watch events with expected annotations")
		for sawAnnotations := false; !sawAnnotations; {
			select {
			case evt, ok := <-epsWatch.ResultChan():
				framework.ExpectEqual(ok, true, "watch channel should not close")
				framework.ExpectEqual(evt.Type, watch.Modified)
				watchedEPS, isEPS := evt.Object.(*discoveryv1.EndpointSlice)
				framework.ExpectEqual(isEPS, true, fmt.Sprintf("expected EndpointSlice, got %T", evt.Object))
				if watchedEPS.Annotations["patched"] == "true" {
					framework.Logf("saw patched and updated annotations")
					sawAnnotations = true
					epsWatch.Stop()
				} else {
					framework.Logf("missing expected annotations, waiting: %#v", watchedEPS.Annotations)
				}
			case <-time.After(wait.ForeverTestTimeout):
				framework.Fail("timed out waiting for watch event")
			}
		}

		ginkgo.By("deleting")

		err = epsClient.Delete(context.TODO(), createdEPS.Name, metav1.DeleteOptions{})
		framework.ExpectNoError(err)
		_, err = epsClient.Get(context.TODO(), createdEPS.Name, metav1.GetOptions{})
		framework.ExpectEqual(apierrors.IsNotFound(err), true, fmt.Sprintf("expected 404, got %v", err))
		epsList, err = epsClient.List(context.TODO(), metav1.ListOptions{LabelSelector: "special-label=" + f.UniqueName})
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(epsList.Items), 2, "filtered list should have 2 items")
		for _, eps := range epsList.Items {
			if eps.Namespace == createdEPS.Namespace && eps.Name == createdEPS.Name {
				framework.Fail("listing after deleting createdEPS")
			}
		}

		ginkgo.By("deleting a collection")
		err = epsClient.DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: "special-label=" + f.UniqueName})
		framework.ExpectNoError(err)
		epsList, err = epsClient.List(context.TODO(), metav1.ListOptions{LabelSelector: "special-label=" + f.UniqueName})
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(epsList.Items), 0, "filtered list should have 0 items")
	})
})

// expectEndpointsAndSlices verifies that Endpoints and EndpointSlices exist for
// a given Service and Namespace with the appropriate attributes set. This is a
// relatively complex function as the order of attributes or resources is not
// necessarily consistent. It is used as a helper function for the tests above
// and takes some shortcuts with the assumption that those test cases will be
// the only caller of this function.
func expectEndpointsAndSlices(cs clientset.Interface, ns string, svc *v1.Service, pods []*v1.Pod, numSubsets, numSlices int, namedPort bool) {
	endpointSlices := []discoveryv1.EndpointSlice{}
	if err := wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		endpointSlicesFound, hasMatchingSlices := hasMatchingEndpointSlices(cs, ns, svc.Name, len(pods), numSlices)
		if !hasMatchingSlices {
			return false, nil
		}
		endpointSlices = endpointSlicesFound
		return true, nil
	}); err != nil {
		framework.Failf("Timed out waiting for EndpointSlices to match expectations: %v", err)
	}

	endpoints := &v1.Endpoints{}
	if err := wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
		endpointsFound, hasMatchingEndpoints := hasMatchingEndpoints(cs, ns, svc.Name, len(pods), numSubsets)
		if !hasMatchingEndpoints {
			framework.Logf("Matching Endpoints not found")
			return false, nil
		}
		endpoints = endpointsFound
		return true, nil
	}); err != nil {
		framework.Failf("Timed out waiting for Endpoints to match expectations: %v", err)
	}

	podsByIP := map[string]*v1.Pod{}
	for _, pod := range pods {
		podsByIP[pod.Status.PodIP] = pod
		if len(pod.Spec.Containers) != 1 {
			framework.Failf("Expected pod to have 1 container, got %d", len(pod.Spec.Containers))
		}
	}

	if endpoints.Name != svc.Name {
		framework.Failf("Expected Endpoints name to be %s, got %s", svc.Name, endpoints.Name)
	}

	totalEndpointAddresses := 0
	for _, subset := range endpoints.Subsets {
		addresses := append(subset.Addresses, subset.NotReadyAddresses...)
		totalEndpointAddresses += len(addresses)

		if len(subset.Ports) != len(svc.Spec.Ports) {
			framework.Failf("Expected subset to have %d ports, got %d", len(svc.Spec.Ports), len(subset.Ports))
		}

		// If not a named port, the subset ports should directly correspond with
		// the Service ports.
		if !namedPort {
			for i, subsetPort := range subset.Ports {
				svcPort := svc.Spec.Ports[i]
				if subsetPort.Name != svcPort.Name {
					framework.Failf("Expected port name to be %s, got %s", svcPort.Name, subsetPort.Name)
				}
				if subsetPort.Protocol != svcPort.Protocol {
					framework.Failf("Expected protocol to be %s, got %s", svcPort.Protocol, subsetPort.Protocol)
				}
				if subsetPort.Port != svcPort.TargetPort.IntVal {
					framework.Failf("Expected port to be %d, got %d", svcPort.TargetPort.IntVal, subsetPort.Port)
				}
			}
		}

		for _, address := range addresses {
			pod, ok := podsByIP[address.IP]
			if !ok {
				framework.Failf("Unexpected address with IP: %s", address.IP)
			}

			ensurePodTargetRef(pod, address.TargetRef)

			// If a named port, the subset ports should directly correspond with
			// each individual pod.
			if namedPort {
				container := pod.Spec.Containers[0]
				for _, port := range container.Ports {
					if port.Name == svc.Spec.Ports[0].TargetPort.String() {
						subsetPort := subset.Ports[0]
						if subsetPort.Port != port.ContainerPort {
							framework.Failf("Expected subset port to be %d, got %d", port.ContainerPort, subsetPort.Port)
						}
						if subsetPort.Name != svc.Spec.Ports[0].Name {
							framework.Failf("Expected subset port name to be %s, got %s", svc.Spec.Ports[0].Name, subsetPort.Name)
						}
					}
				}
			}
		}
	}

	if len(pods) != totalEndpointAddresses {
		framework.Failf("Expected %d addresses, got %d", len(pods), totalEndpointAddresses)
	}

	if len(pods) == 0 && len(endpointSlices) != 1 {
		framework.Failf("Expected 1 EndpointSlice, got %d", len(endpointSlices))
	}

	// Use a set for deduping values. Duplicate addresses are technically valid
	// here although rare.
	esAddresses := sets.NewString()
	for _, endpointSlice := range endpointSlices {
		for _, endpoint := range endpointSlice.Endpoints {
			esAddresses.Insert(endpoint.Addresses[0])
		}
		if len(pods) == 0 && len(endpointSlice.Ports) != 0 {
			framework.Failf("Expected EndpointSlice to have 0 ports, got %d", len(endpointSlice.Ports))
		}
		if len(pods) > 0 && len(endpointSlice.Ports) != len(svc.Spec.Ports) {
			framework.Failf("Expected EndpointSlice to have %d ports, got %d", len(svc.Spec.Ports), len(endpointSlice.Ports))
		}

		// If not a named port, the EndpointSlice ports should directly
		// correspond with the Service ports.
		if !namedPort {
			for i, esPort := range endpointSlice.Ports {
				svcPort := svc.Spec.Ports[i]
				if *esPort.Name != svcPort.Name {
					framework.Failf("Expected port name to be %s, got %s", svcPort.Name, *esPort.Name)
				}
				if *esPort.Protocol != svcPort.Protocol {
					framework.Failf("Expected protocol to be %s, got %s", svcPort.Protocol, *esPort.Protocol)
				}
				if *esPort.Port != svcPort.TargetPort.IntVal {
					framework.Failf("Expected port to be %d, got %d", svcPort.TargetPort.IntVal, *esPort.Port)
				}
			}
		}

		for _, endpoint := range endpointSlice.Endpoints {
			if len(endpoint.Addresses) == 0 {
				framework.Failf("Expected EndpointSlice endpoint to have at least 1 address")
			}
			pod, ok := podsByIP[endpoint.Addresses[0]]
			if !ok {
				framework.Failf("Unexpected address with IP: %s", endpoint.Addresses[0])
			}

			ensurePodTargetRef(pod, endpoint.TargetRef)

			// If a named port, the EndpointSlice ports should directly
			// correspond with each individual pod.
			if namedPort {
				container := pod.Spec.Containers[0]
				for _, port := range container.Ports {
					if port.Name == svc.Spec.Ports[0].TargetPort.String() {
						esPort := endpointSlice.Ports[0]
						if *esPort.Port != port.ContainerPort {
							framework.Failf("Expected EndpointSlice port to be %d, got %d", port.ContainerPort, *esPort.Port)
						}
						if *esPort.Name != svc.Spec.Ports[0].Name {
							framework.Failf("Expected EndpointSlice port name to be %s, got %s", svc.Spec.Ports[0].Name, *esPort.Name)
						}
					}
				}
			}
		}
	}

	if len(pods) != esAddresses.Len() {
		framework.Failf("Expected %d addresses, got %d", len(pods), esAddresses.Len())
	}
}

// deleteEndpointSlices deletes EndpointSlices for the specified Service.
func deleteEndpointSlices(cs clientset.Interface, ns string, svc *v1.Service) {
	listOptions := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", discoveryv1.LabelServiceName, svc.Name)}
	esList, err := cs.DiscoveryV1().EndpointSlices(ns).List(context.TODO(), listOptions)
	framework.ExpectNoError(err, "Error fetching EndpointSlices for %s/%s Service", ns, svc.Name)

	for _, endpointSlice := range esList.Items {
		err := cs.DiscoveryV1().EndpointSlices(ns).Delete(context.TODO(), endpointSlice.Name, metav1.DeleteOptions{})
		framework.ExpectNoError(err, "Error deleting %s/%s EndpointSlice", ns, endpointSlice.Name)
	}
}

// hasMatchingEndpointSlices returns any EndpointSlices that match the
// conditions along with a boolean indicating if all the conditions have been
// met.
func hasMatchingEndpointSlices(cs clientset.Interface, ns, svcName string, numEndpoints, numSlices int) ([]discoveryv1.EndpointSlice, bool) {
	listOptions := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", discoveryv1.LabelServiceName, svcName)}
	esList, err := cs.DiscoveryV1().EndpointSlices(ns).List(context.TODO(), listOptions)
	framework.ExpectNoError(err, "Error fetching EndpointSlice for Service %s/%s", ns, svcName)

	if len(esList.Items) == 0 {
		framework.Logf("EndpointSlice for Service %s/%s not found", ns, svcName)
		return []discoveryv1.EndpointSlice{}, false
	}
	// In some cases the EndpointSlice controller will create more
	// EndpointSlices than necessary resulting in some duplication. This is
	// valid and tests should only fail here if less EndpointSlices than
	// expected are added.
	if len(esList.Items) < numSlices {
		framework.Logf("Expected at least %d EndpointSlices for Service %s/%s, got %d", numSlices, ns, svcName, len(esList.Items))
		for i, epSlice := range esList.Items {
			epsData, err := json.Marshal(epSlice)
			if err != nil {
				framework.Logf("Error marshaling JSON for EndpointSlice: %v", err)
			} else {
				framework.Logf("%d - %v", i, string(epsData))
			}
		}
		return esList.Items, false
	}

	actualNumEndpoints := 0
	for _, endpointSlice := range esList.Items {
		actualNumEndpoints += len(endpointSlice.Endpoints)
	}
	// In some cases the EndpointSlice controller will create more
	// EndpointSlices than necessary resulting in some duplication. This is
	// valid and tests should only fail here if less EndpointSlices than
	// expected are added.
	if actualNumEndpoints < numEndpoints {
		framework.Logf("EndpointSlices for %s/%s Service have %d/%d endpoints", ns, svcName, actualNumEndpoints, numEndpoints)
		return esList.Items, false
	}

	return esList.Items, true
}

// hasMatchingEndpoints returns any Endpoints that match the conditions along
// with a boolean indicating if all the conditions have been met.
func hasMatchingEndpoints(cs clientset.Interface, ns, svcName string, numIPs, numSubsets int) (*v1.Endpoints, bool) {
	endpoints, err := cs.CoreV1().Endpoints(ns).Get(context.TODO(), svcName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			framework.Logf("Endpoints for %s/%s Service not found", ns, svcName)
			return nil, false
		}
		framework.ExpectNoError(err, "Error fetching Endpoints for %s/%s Service", ns, svcName)
	}
	if len(endpoints.Subsets) != numSubsets {
		framework.Logf("Endpoints for %s/%s Service with %d/%d Subsets", ns, svcName, len(endpoints.Subsets), numSubsets)
		return nil, false
	}

	actualNumIPs := 0
	for _, endpointSubset := range endpoints.Subsets {
		actualNumIPs += len(endpointSubset.Addresses) + len(endpointSubset.NotReadyAddresses)
	}
	if actualNumIPs != numIPs {
		framework.Logf("Endpoints for %s/%s Service with %d/%d IPs", ns, svcName, actualNumIPs, numIPs)
		return nil, false
	}

	return endpoints, true
}

// ensurePodTargetRef ensures that a Pod matches the provided target reference.
func ensurePodTargetRef(pod *v1.Pod, targetRef *v1.ObjectReference) {
	if targetRef == nil {
		framework.Failf("Expected TargetRef to not be nil")
	}
	if targetRef.Kind != "Pod" {
		framework.Failf("Expected TargetRef.Kind to be Pod, got %s", targetRef.Kind)
	}
	if targetRef.Namespace != pod.Namespace {
		framework.Failf("Expected TargetRef.Namespace to be %s, got %s", pod.Namespace, targetRef.Namespace)
	}
	if targetRef.Name != pod.Name {
		framework.Failf("Expected TargetRef.Name to be %s, got %s", pod.Name, targetRef.Name)
	}
	if targetRef.UID != pod.UID {
		framework.Failf("Expected TargetRef.UID to be %s, got %s", pod.UID, targetRef.UID)
	}
}

// createServiceReportErr creates a Service and reports any associated error.
func createServiceReportErr(cs clientset.Interface, ns string, service *v1.Service) *v1.Service {
	svc, err := cs.CoreV1().Services(ns).Create(context.TODO(), service, metav1.CreateOptions{})
	framework.ExpectNoError(err, "error deleting Service")
	return svc
}
