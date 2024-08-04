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

package autoscaling

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubectl "k8s.io/kubernetes/test/e2e/framework/kubectl"
	e2erc "k8s.io/kubernetes/test/e2e/framework/rc"
	e2eresource "k8s.io/kubernetes/test/e2e/framework/resource"
	e2eservice "k8s.io/kubernetes/test/e2e/framework/service"
	testutils "k8s.io/kubernetes/test/utils"

	"github.com/onsi/ginkgo/v2"

	scaleclient "k8s.io/client-go/scale"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

const (
	dynamicConsumptionTimeInSeconds = 30
	dynamicRequestSizeInMillicores  = 100
	dynamicRequestSizeInMegabytes   = 100
	dynamicRequestSizeCustomMetric  = 10
	port                            = 80
	targetPort                      = 8080
	sidecarTargetPort               = 8081
	timeoutRC                       = 120 * time.Second
	startServiceTimeout             = time.Minute
	startServiceInterval            = 5 * time.Second
	rcIsNil                         = "ERROR: replicationController = nil"
	deploymentIsNil                 = "ERROR: deployment = nil"
	rsIsNil                         = "ERROR: replicaset = nil"
	invalidKind                     = "ERROR: invalid workload kind for resource consumer"
	customMetricName                = "QPS"
	serviceInitializationTimeout    = 2 * time.Minute
	serviceInitializationInterval   = 15 * time.Second
)

var (
	resourceConsumerImage = imageutils.GetE2EImage(imageutils.ResourceConsumer)
)

var (
	// KindRC is the GVK for ReplicationController
	KindRC = schema.GroupVersionKind{Version: "v1", Kind: "ReplicationController"}
	// KindDeployment is the GVK for Deployment
	KindDeployment = schema.GroupVersionKind{Group: "apps", Version: "v1beta2", Kind: "Deployment"}
	// KindReplicaSet is the GVK for ReplicaSet
	KindReplicaSet = schema.GroupVersionKind{Group: "apps", Version: "v1beta2", Kind: "ReplicaSet"}
)

// ScalingDirection identifies the scale direction for HPA Behavior.
type ScalingDirection int

const (
	DirectionUnknown ScalingDirection = iota
	ScaleUpDirection
	ScaleDownDirection
)

/*
ResourceConsumer is a tool for testing. It helps create specified usage of CPU or memory (Warning: memory not supported)
typical use case:
rc.ConsumeCPU(600)
// ... check your assumption here
rc.ConsumeCPU(300)
// ... check your assumption here
*/
type ResourceConsumer struct {
	name                     string
	controllerName           string
	kind                     schema.GroupVersionKind
	nsName                   string
	clientSet                clientset.Interface
	scaleClient              scaleclient.ScalesGetter
	cpu                      chan int
	mem                      chan int
	customMetric             chan int
	stopCPU                  chan int
	stopMem                  chan int
	stopCustomMetric         chan int
	stopWaitGroup            sync.WaitGroup
	consumptionTimeInSeconds int
	sleepTime                time.Duration
	requestSizeInMillicores  int
	requestSizeInMegabytes   int
	requestSizeCustomMetric  int
	sidecarStatus            SidecarStatusType
	sidecarType              SidecarWorkloadType
}

// NewDynamicResourceConsumer is a wrapper to create a new dynamic ResourceConsumer
func NewDynamicResourceConsumer(name, nsName string, kind schema.GroupVersionKind, replicas, initCPUTotal, initMemoryTotal, initCustomMetric int, cpuLimit, memLimit int64, clientset clientset.Interface, scaleClient scaleclient.ScalesGetter, enableSidecar SidecarStatusType, sidecarType SidecarWorkloadType) *ResourceConsumer {
	return newResourceConsumer(name, nsName, kind, replicas, initCPUTotal, initMemoryTotal, initCustomMetric, dynamicConsumptionTimeInSeconds,
		dynamicRequestSizeInMillicores, dynamicRequestSizeInMegabytes, dynamicRequestSizeCustomMetric, cpuLimit, memLimit, clientset, scaleClient, nil, nil, enableSidecar, sidecarType)
}

// getSidecarContainer returns sidecar container
func getSidecarContainer(name string, cpuLimit, memLimit int64) v1.Container {
	container := v1.Container{
		Name:    name + "-sidecar",
		Image:   resourceConsumerImage,
		Command: []string{"/consumer", "-port=8081"},
		Ports:   []v1.ContainerPort{{ContainerPort: 80}},
	}

	if cpuLimit > 0 || memLimit > 0 {
		container.Resources.Limits = v1.ResourceList{}
		container.Resources.Requests = v1.ResourceList{}
	}

	if cpuLimit > 0 {
		container.Resources.Limits[v1.ResourceCPU] = *resource.NewMilliQuantity(cpuLimit, resource.DecimalSI)
		container.Resources.Requests[v1.ResourceCPU] = *resource.NewMilliQuantity(cpuLimit, resource.DecimalSI)
	}

	if memLimit > 0 {
		container.Resources.Limits[v1.ResourceMemory] = *resource.NewQuantity(memLimit*1024*1024, resource.DecimalSI)
		container.Resources.Requests[v1.ResourceMemory] = *resource.NewQuantity(memLimit*1024*1024, resource.DecimalSI)
	}

	return container
}

/*
NewResourceConsumer creates new ResourceConsumer
initCPUTotal argument is in millicores
initMemoryTotal argument is in megabytes
memLimit argument is in megabytes, memLimit is a maximum amount of memory that can be consumed by a single pod
cpuLimit argument is in millicores, cpuLimit is a maximum amount of cpu that can be consumed by a single pod
*/
func newResourceConsumer(name, nsName string, kind schema.GroupVersionKind, replicas, initCPUTotal, initMemoryTotal, initCustomMetric, consumptionTimeInSeconds, requestSizeInMillicores,
	requestSizeInMegabytes int, requestSizeCustomMetric int, cpuLimit, memLimit int64, clientset clientset.Interface, scaleClient scaleclient.ScalesGetter, podAnnotations, serviceAnnotations map[string]string, sidecarStatus SidecarStatusType, sidecarType SidecarWorkloadType) *ResourceConsumer {
	if podAnnotations == nil {
		podAnnotations = make(map[string]string)
	}
	if serviceAnnotations == nil {
		serviceAnnotations = make(map[string]string)
	}

	var additionalContainers []v1.Container

	if sidecarStatus == Enable {
		sidecarContainer := getSidecarContainer(name, cpuLimit, memLimit)
		additionalContainers = append(additionalContainers, sidecarContainer)
	}

	runServiceAndWorkloadForResourceConsumer(clientset, nsName, name, kind, replicas, cpuLimit, memLimit, podAnnotations, serviceAnnotations, additionalContainers)
	controllerName := name + "-ctrl"
	// If sidecar is enabled and busy, run service and consumer for sidecar
	if sidecarStatus == Enable && sidecarType == Busy {
		runServiceAndSidecarForResourceConsumer(clientset, nsName, name, kind, replicas, serviceAnnotations)
		controllerName = name + "-sidecar-ctrl"
	}

	rc := &ResourceConsumer{
		name:                     name,
		controllerName:           controllerName,
		kind:                     kind,
		nsName:                   nsName,
		clientSet:                clientset,
		scaleClient:              scaleClient,
		cpu:                      make(chan int),
		mem:                      make(chan int),
		customMetric:             make(chan int),
		stopCPU:                  make(chan int),
		stopMem:                  make(chan int),
		stopCustomMetric:         make(chan int),
		consumptionTimeInSeconds: consumptionTimeInSeconds,
		sleepTime:                time.Duration(consumptionTimeInSeconds) * time.Second,
		requestSizeInMillicores:  requestSizeInMillicores,
		requestSizeInMegabytes:   requestSizeInMegabytes,
		requestSizeCustomMetric:  requestSizeCustomMetric,
		sidecarType:              sidecarType,
		sidecarStatus:            sidecarStatus,
	}

	go rc.makeConsumeCPURequests()
	rc.ConsumeCPU(initCPUTotal)
	go rc.makeConsumeMemRequests()
	rc.ConsumeMem(initMemoryTotal)
	go rc.makeConsumeCustomMetric()
	rc.ConsumeCustomMetric(initCustomMetric)
	return rc
}

// ConsumeCPU consumes given number of CPU
func (rc *ResourceConsumer) ConsumeCPU(millicores int) {
	framework.Logf("RC %s: consume %v millicores in total", rc.name, millicores)
	rc.cpu <- millicores
}

// ConsumeMem consumes given number of Mem
func (rc *ResourceConsumer) ConsumeMem(megabytes int) {
	framework.Logf("RC %s: consume %v MB in total", rc.name, megabytes)
	rc.mem <- megabytes
}

// ConsumeCustomMetric consumes given number of custom metric
func (rc *ResourceConsumer) ConsumeCustomMetric(amount int) {
	framework.Logf("RC %s: consume custom metric %v in total", rc.name, amount)
	rc.customMetric <- amount
}

func (rc *ResourceConsumer) makeConsumeCPURequests() {
	defer ginkgo.GinkgoRecover()
	rc.stopWaitGroup.Add(1)
	defer rc.stopWaitGroup.Done()
	tick := time.After(time.Duration(0))
	millicores := 0
	for {
		select {
		case millicores = <-rc.cpu:
			if millicores != 0 {
				framework.Logf("RC %s: setting consumption to %v millicores in total", rc.name, millicores)
			} else {
				framework.Logf("RC %s: disabling CPU consumption", rc.name)
			}
		case <-tick:
			if millicores != 0 {
				framework.Logf("RC %s: sending request to consume %d millicores", rc.name, millicores)
				rc.sendConsumeCPURequest(millicores)
			}
			tick = time.After(rc.sleepTime)
		case <-rc.stopCPU:
			framework.Logf("RC %s: stopping CPU consumer", rc.name)
			return
		}
	}
}

func (rc *ResourceConsumer) makeConsumeMemRequests() {
	defer ginkgo.GinkgoRecover()
	rc.stopWaitGroup.Add(1)
	defer rc.stopWaitGroup.Done()
	tick := time.After(time.Duration(0))
	megabytes := 0
	for {
		select {
		case megabytes = <-rc.mem:
			if megabytes != 0 {
				framework.Logf("RC %s: setting consumption to %v MB in total", rc.name, megabytes)
			} else {
				framework.Logf("RC %s: disabling mem consumption", rc.name)
			}
		case <-tick:
			if megabytes != 0 {
				framework.Logf("RC %s: sending request to consume %d MB", rc.name, megabytes)
				rc.sendConsumeMemRequest(megabytes)
			}
			tick = time.After(rc.sleepTime)
		case <-rc.stopMem:
			framework.Logf("RC %s: stopping mem consumer", rc.name)
			return
		}
	}
}

func (rc *ResourceConsumer) makeConsumeCustomMetric() {
	defer ginkgo.GinkgoRecover()
	rc.stopWaitGroup.Add(1)
	defer rc.stopWaitGroup.Done()
	tick := time.After(time.Duration(0))
	delta := 0
	for {
		select {
		case delta = <-rc.customMetric:
			if delta != 0 {
				framework.Logf("RC %s: setting bump of metric %s to %d in total", rc.name, customMetricName, delta)
			} else {
				framework.Logf("RC %s: disabling consumption of custom metric %s", rc.name, customMetricName)
			}
		case <-tick:
			if delta != 0 {
				framework.Logf("RC %s: sending request to consume %d of custom metric %s", rc.name, delta, customMetricName)
				rc.sendConsumeCustomMetric(delta)
			}
			tick = time.After(rc.sleepTime)
		case <-rc.stopCustomMetric:
			framework.Logf("RC %s: stopping metric consumer", rc.name)
			return
		}
	}
}

func (rc *ResourceConsumer) sendConsumeCPURequest(millicores int) {
	ctx, cancel := context.WithTimeout(context.Background(), framework.SingleCallTimeout)
	defer cancel()

	err := wait.PollImmediate(serviceInitializationInterval, serviceInitializationTimeout, func() (bool, error) {
		proxyRequest, err := e2eservice.GetServicesProxyRequest(rc.clientSet, rc.clientSet.CoreV1().RESTClient().Post())
		framework.ExpectNoError(err)
		req := proxyRequest.Namespace(rc.nsName).
			Name(rc.controllerName).
			Suffix("ConsumeCPU").
			Param("millicores", strconv.Itoa(millicores)).
			Param("durationSec", strconv.Itoa(rc.consumptionTimeInSeconds)).
			Param("requestSizeMillicores", strconv.Itoa(rc.requestSizeInMillicores))
		framework.Logf("ConsumeCPU URL: %v", *req.URL())
		_, err = req.DoRaw(ctx)
		if err != nil {
			framework.Logf("ConsumeCPU failure: %v", err)
			return false, nil
		}
		return true, nil
	})

	framework.ExpectNoError(err)
}

// sendConsumeMemRequest sends POST request for memory consumption
func (rc *ResourceConsumer) sendConsumeMemRequest(megabytes int) {
	ctx, cancel := context.WithTimeout(context.Background(), framework.SingleCallTimeout)
	defer cancel()

	err := wait.PollImmediate(serviceInitializationInterval, serviceInitializationTimeout, func() (bool, error) {
		proxyRequest, err := e2eservice.GetServicesProxyRequest(rc.clientSet, rc.clientSet.CoreV1().RESTClient().Post())
		framework.ExpectNoError(err)
		req := proxyRequest.Namespace(rc.nsName).
			Name(rc.controllerName).
			Suffix("ConsumeMem").
			Param("megabytes", strconv.Itoa(megabytes)).
			Param("durationSec", strconv.Itoa(rc.consumptionTimeInSeconds)).
			Param("requestSizeMegabytes", strconv.Itoa(rc.requestSizeInMegabytes))
		framework.Logf("ConsumeMem URL: %v", *req.URL())
		_, err = req.DoRaw(ctx)
		if err != nil {
			framework.Logf("ConsumeMem failure: %v", err)
			return false, nil
		}
		return true, nil
	})

	framework.ExpectNoError(err)
}

// sendConsumeCustomMetric sends POST request for custom metric consumption
func (rc *ResourceConsumer) sendConsumeCustomMetric(delta int) {
	ctx, cancel := context.WithTimeout(context.Background(), framework.SingleCallTimeout)
	defer cancel()

	err := wait.PollImmediate(serviceInitializationInterval, serviceInitializationTimeout, func() (bool, error) {
		proxyRequest, err := e2eservice.GetServicesProxyRequest(rc.clientSet, rc.clientSet.CoreV1().RESTClient().Post())
		framework.ExpectNoError(err)
		req := proxyRequest.Namespace(rc.nsName).
			Name(rc.controllerName).
			Suffix("BumpMetric").
			Param("metric", customMetricName).
			Param("delta", strconv.Itoa(delta)).
			Param("durationSec", strconv.Itoa(rc.consumptionTimeInSeconds)).
			Param("requestSizeMetrics", strconv.Itoa(rc.requestSizeCustomMetric))
		framework.Logf("ConsumeCustomMetric URL: %v", *req.URL())
		_, err = req.DoRaw(ctx)
		if err != nil {
			framework.Logf("ConsumeCustomMetric failure: %v", err)
			return false, nil
		}
		return true, nil
	})
	framework.ExpectNoError(err)
}

// GetReplicas get the replicas
func (rc *ResourceConsumer) GetReplicas() int {
	switch rc.kind {
	case KindRC:
		replicationController, err := rc.clientSet.CoreV1().ReplicationControllers(rc.nsName).Get(context.TODO(), rc.name, metav1.GetOptions{})
		framework.ExpectNoError(err)
		if replicationController == nil {
			framework.Failf(rcIsNil)
		}
		return int(replicationController.Status.ReadyReplicas)
	case KindDeployment:
		deployment, err := rc.clientSet.AppsV1().Deployments(rc.nsName).Get(context.TODO(), rc.name, metav1.GetOptions{})
		framework.ExpectNoError(err)
		if deployment == nil {
			framework.Failf(deploymentIsNil)
		}
		return int(deployment.Status.ReadyReplicas)
	case KindReplicaSet:
		rs, err := rc.clientSet.AppsV1().ReplicaSets(rc.nsName).Get(context.TODO(), rc.name, metav1.GetOptions{})
		framework.ExpectNoError(err)
		if rs == nil {
			framework.Failf(rsIsNil)
		}
		return int(rs.Status.ReadyReplicas)
	default:
		framework.Failf(invalidKind)
	}
	return 0
}

// GetHpa get the corresponding horizontalPodAutoscaler object
func (rc *ResourceConsumer) GetHpa(name string) (*autoscalingv1.HorizontalPodAutoscaler, error) {
	return rc.clientSet.AutoscalingV1().HorizontalPodAutoscalers(rc.nsName).Get(context.TODO(), name, metav1.GetOptions{})
}

// WaitForReplicas wait for the desired replicas
func (rc *ResourceConsumer) WaitForReplicas(desiredReplicas int, duration time.Duration) {
	interval := 20 * time.Second
	err := wait.PollImmediate(interval, duration, func() (bool, error) {
		replicas := rc.GetReplicas()
		framework.Logf("waiting for %d replicas (current: %d)", desiredReplicas, replicas)
		return replicas == desiredReplicas, nil // Expected number of replicas found. Exit.
	})
	framework.ExpectNoErrorWithOffset(1, err, "timeout waiting %v for %d replicas", duration, desiredReplicas)
}

// EnsureDesiredReplicasInRange ensure the replicas is in a desired range
func (rc *ResourceConsumer) EnsureDesiredReplicasInRange(minDesiredReplicas, maxDesiredReplicas int, duration time.Duration, hpaName string) {
	interval := 10 * time.Second
	err := wait.PollImmediate(interval, duration, func() (bool, error) {
		replicas := rc.GetReplicas()
		framework.Logf("expecting there to be in [%d, %d] replicas (are: %d)", minDesiredReplicas, maxDesiredReplicas, replicas)
		as, err := rc.GetHpa(hpaName)
		if err != nil {
			framework.Logf("Error getting HPA: %s", err)
		} else {
			framework.Logf("HPA status: %+v", as.Status)
		}
		if replicas < minDesiredReplicas {
			return false, fmt.Errorf("number of replicas below target")
		} else if replicas > maxDesiredReplicas {
			return false, fmt.Errorf("number of replicas above target")
		} else {
			return false, nil // Expected number of replicas found. Continue polling until timeout.
		}
	})
	// The call above always returns an error, but if it is timeout, it's OK (condition satisfied all the time).
	if err == wait.ErrWaitTimeout {
		framework.Logf("Number of replicas was stable over %v", duration)
		return
	}
	framework.ExpectNoErrorWithOffset(1, err)
}

// Pause stops background goroutines responsible for consuming resources.
func (rc *ResourceConsumer) Pause() {
	ginkgo.By(fmt.Sprintf("HPA pausing RC %s", rc.name))
	rc.stopCPU <- 0
	rc.stopMem <- 0
	rc.stopCustomMetric <- 0
	rc.stopWaitGroup.Wait()
}

// Resume starts background goroutines responsible for consuming resources.
func (rc *ResourceConsumer) Resume() {
	ginkgo.By(fmt.Sprintf("HPA resuming RC %s", rc.name))
	go rc.makeConsumeCPURequests()
	go rc.makeConsumeMemRequests()
	go rc.makeConsumeCustomMetric()
}

// CleanUp clean up the background goroutines responsible for consuming resources.
func (rc *ResourceConsumer) CleanUp() {
	ginkgo.By(fmt.Sprintf("Removing consuming RC %s", rc.name))
	close(rc.stopCPU)
	close(rc.stopMem)
	close(rc.stopCustomMetric)
	rc.stopWaitGroup.Wait()
	// Wait some time to ensure all child goroutines are finished.
	time.Sleep(10 * time.Second)
	kind := rc.kind.GroupKind()
	framework.ExpectNoError(e2eresource.DeleteResourceAndWaitForGC(rc.clientSet, kind, rc.nsName, rc.name))
	framework.ExpectNoError(rc.clientSet.CoreV1().Services(rc.nsName).Delete(context.TODO(), rc.name, metav1.DeleteOptions{}))
	framework.ExpectNoError(e2eresource.DeleteResourceAndWaitForGC(rc.clientSet, schema.GroupKind{Kind: "ReplicationController"}, rc.nsName, rc.controllerName))
	framework.ExpectNoError(rc.clientSet.CoreV1().Services(rc.nsName).Delete(context.TODO(), rc.name+"-ctrl", metav1.DeleteOptions{}))
	// Cleanup sidecar related resources
	if rc.sidecarStatus == Enable && rc.sidecarType == Busy {
		framework.ExpectNoError(rc.clientSet.CoreV1().Services(rc.nsName).Delete(context.TODO(), rc.name+"-sidecar", metav1.DeleteOptions{}))
		framework.ExpectNoError(rc.clientSet.CoreV1().Services(rc.nsName).Delete(context.TODO(), rc.name+"-sidecar-ctrl", metav1.DeleteOptions{}))
	}
}

func createService(c clientset.Interface, name, ns string, annotations, selectors map[string]string, port int32, targetPort int) (*v1.Service, error) {
	return c.CoreV1().Services(ns).Create(context.TODO(), &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port:       port,
				TargetPort: intstr.FromInt(targetPort),
			}},
			Selector: selectors,
		},
	}, metav1.CreateOptions{})
}

// runServiceAndSidecarForResourceConsumer creates service and runs resource consumer for sidecar container
func runServiceAndSidecarForResourceConsumer(c clientset.Interface, ns, name string, kind schema.GroupVersionKind, replicas int, serviceAnnotations map[string]string) {
	ginkgo.By(fmt.Sprintf("Running consuming RC sidecar %s via %s with %v replicas", name, kind, replicas))

	sidecarName := name + "-sidecar"
	serviceSelectors := map[string]string{
		"name": name,
	}
	_, err := createService(c, sidecarName, ns, serviceAnnotations, serviceSelectors, port, sidecarTargetPort)
	framework.ExpectNoError(err)

	ginkgo.By(fmt.Sprintf("Running controller for sidecar"))
	controllerName := sidecarName + "-ctrl"
	_, err = createService(c, controllerName, ns, map[string]string{}, map[string]string{"name": controllerName}, port, targetPort)
	framework.ExpectNoError(err)

	dnsClusterFirst := v1.DNSClusterFirst
	controllerRcConfig := testutils.RCConfig{
		Client:    c,
		Image:     imageutils.GetE2EImage(imageutils.Agnhost),
		Name:      controllerName,
		Namespace: ns,
		Timeout:   timeoutRC,
		Replicas:  1,
		Command:   []string{"/agnhost", "resource-consumer-controller", "--consumer-service-name=" + sidecarName, "--consumer-service-namespace=" + ns, "--consumer-port=80"},
		DNSPolicy: &dnsClusterFirst,
	}

	framework.ExpectNoError(e2erc.RunRC(controllerRcConfig))
	// Wait for endpoints to propagate for the controller service.
	framework.ExpectNoError(framework.WaitForServiceEndpointsNum(
		c, ns, controllerName, 1, startServiceInterval, startServiceTimeout))
}

func runServiceAndWorkloadForResourceConsumer(c clientset.Interface, ns, name string, kind schema.GroupVersionKind, replicas int, cpuLimitMillis, memLimitMb int64, podAnnotations, serviceAnnotations map[string]string, additionalContainers []v1.Container) {
	ginkgo.By(fmt.Sprintf("Running consuming RC %s via %s with %v replicas", name, kind, replicas))
	_, err := createService(c, name, ns, serviceAnnotations, map[string]string{"name": name}, port, targetPort)
	framework.ExpectNoError(err)

	rcConfig := testutils.RCConfig{
		Client:               c,
		Image:                resourceConsumerImage,
		Name:                 name,
		Namespace:            ns,
		Timeout:              timeoutRC,
		Replicas:             replicas,
		CpuRequest:           cpuLimitMillis,
		CpuLimit:             cpuLimitMillis,
		MemRequest:           memLimitMb * 1024 * 1024, // MemLimit is in bytes
		MemLimit:             memLimitMb * 1024 * 1024,
		Annotations:          podAnnotations,
		AdditionalContainers: additionalContainers,
	}

	switch kind {
	case KindRC:
		framework.ExpectNoError(e2erc.RunRC(rcConfig))
	case KindDeployment:
		dpConfig := testutils.DeploymentConfig{
			RCConfig: rcConfig,
		}
		ginkgo.By(fmt.Sprintf("creating deployment %s in namespace %s", dpConfig.Name, dpConfig.Namespace))
		dpConfig.NodeDumpFunc = framework.DumpNodeDebugInfo
		dpConfig.ContainerDumpFunc = e2ekubectl.LogFailedContainers
		framework.ExpectNoError(testutils.RunDeployment(dpConfig))
	case KindReplicaSet:
		rsConfig := testutils.ReplicaSetConfig{
			RCConfig: rcConfig,
		}
		ginkgo.By(fmt.Sprintf("creating replicaset %s in namespace %s", rsConfig.Name, rsConfig.Namespace))
		framework.ExpectNoError(runReplicaSet(rsConfig))
	default:
		framework.Failf(invalidKind)
	}

	ginkgo.By(fmt.Sprintf("Running controller"))
	controllerName := name + "-ctrl"
	_, err = createService(c, controllerName, ns, map[string]string{}, map[string]string{"name": controllerName}, port, targetPort)
	framework.ExpectNoError(err)

	dnsClusterFirst := v1.DNSClusterFirst
	controllerRcConfig := testutils.RCConfig{
		Client:    c,
		Image:     imageutils.GetE2EImage(imageutils.Agnhost),
		Name:      controllerName,
		Namespace: ns,
		Timeout:   timeoutRC,
		Replicas:  1,
		Command:   []string{"/agnhost", "resource-consumer-controller", "--consumer-service-name=" + name, "--consumer-service-namespace=" + ns, "--consumer-port=80"},
		DNSPolicy: &dnsClusterFirst,
	}

	framework.ExpectNoError(e2erc.RunRC(controllerRcConfig))
	// Wait for endpoints to propagate for the controller service.
	framework.ExpectNoError(framework.WaitForServiceEndpointsNum(
		c, ns, controllerName, 1, startServiceInterval, startServiceTimeout))
}

// CreateCPUHorizontalPodAutoscaler create a horizontalPodAutoscaler with CPU target
// for consuming resources.
func CreateCPUHorizontalPodAutoscaler(rc *ResourceConsumer, cpu, minReplicas, maxRepl int32) *autoscalingv1.HorizontalPodAutoscaler {
	hpa := &autoscalingv1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rc.name,
			Namespace: rc.nsName,
		},
		Spec: autoscalingv1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: rc.kind.GroupVersion().String(),
				Kind:       rc.kind.Kind,
				Name:       rc.name,
			},
			MinReplicas:                    &minReplicas,
			MaxReplicas:                    maxRepl,
			TargetCPUUtilizationPercentage: &cpu,
		},
	}
	hpa, errHPA := rc.clientSet.AutoscalingV1().HorizontalPodAutoscalers(rc.nsName).Create(context.TODO(), hpa, metav1.CreateOptions{})
	framework.ExpectNoError(errHPA)
	return hpa
}

// DeleteHorizontalPodAutoscaler delete the horizontalPodAutoscaler for consuming resources.
func DeleteHorizontalPodAutoscaler(rc *ResourceConsumer, autoscalerName string) {
	rc.clientSet.AutoscalingV1().HorizontalPodAutoscalers(rc.nsName).Delete(context.TODO(), autoscalerName, metav1.DeleteOptions{})
}

// runReplicaSet launches (and verifies correctness) of a replicaset.
func runReplicaSet(config testutils.ReplicaSetConfig) error {
	ginkgo.By(fmt.Sprintf("creating replicaset %s in namespace %s", config.Name, config.Namespace))
	config.NodeDumpFunc = framework.DumpNodeDebugInfo
	config.ContainerDumpFunc = e2ekubectl.LogFailedContainers
	return testutils.RunReplicaSet(config)
}

// CreateContainerResourceCPUHorizontalPodAutoscaler create a horizontal pod autoscaler with container resource target
// for consuming resources.
func CreateContainerResourceCPUHorizontalPodAutoscaler(rc *ResourceConsumer, cpu, minReplicas, maxRepl int32) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rc.name,
			Namespace: rc.nsName,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: rc.kind.GroupVersion().String(),
				Kind:       rc.kind.Kind,
				Name:       rc.name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxRepl,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: "ContainerResource",
					ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
						Name:      "cpu",
						Container: rc.name,
						Target: autoscalingv2.MetricTarget{
							Type:               "Utilization",
							AverageUtilization: &cpu,
						},
					},
				},
			},
		},
	}
	hpa, errHPA := rc.clientSet.AutoscalingV2().HorizontalPodAutoscalers(rc.nsName).Create(context.TODO(), hpa, metav1.CreateOptions{})
	framework.ExpectNoError(errHPA)
	return hpa
}

// DeleteContainerResourceHPA delete the horizontalPodAutoscaler for consuming resources.
func DeleteContainerResourceHPA(rc *ResourceConsumer, autoscalerName string) {
	rc.clientSet.AutoscalingV2().HorizontalPodAutoscalers(rc.nsName).Delete(context.TODO(), autoscalerName, metav1.DeleteOptions{})
}

func CreateCPUHorizontalPodAutoscalerWithBehavior(rc *ResourceConsumer, cpu int32, minReplicas int32, maxRepl int32, behavior *autoscalingv2.HorizontalPodAutoscalerBehavior) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rc.name,
			Namespace: rc.nsName,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: rc.kind.GroupVersion().String(),
				Kind:       rc.kind.Kind,
				Name:       rc.name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxRepl,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: v1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &cpu,
						},
					},
				},
			},
			Behavior: behavior,
		},
	}
	hpa, errHPA := rc.clientSet.AutoscalingV2().HorizontalPodAutoscalers(rc.nsName).Create(context.TODO(), hpa, metav1.CreateOptions{})
	framework.ExpectNoError(errHPA)
	return hpa
}

func HPABehaviorWithScaleUpAndDownRules(scaleUpRule, scaleDownRule *autoscalingv2.HPAScalingRules) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	return &autoscalingv2.HorizontalPodAutoscalerBehavior{
		ScaleUp:   scaleUpRule,
		ScaleDown: scaleDownRule,
	}
}

func HPABehaviorWithScalingRuleInDirection(scalingDirection ScalingDirection, rule *autoscalingv2.HPAScalingRules) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	var scaleUpRule, scaleDownRule *autoscalingv2.HPAScalingRules
	if scalingDirection == ScaleUpDirection {
		scaleUpRule = rule
	}
	if scalingDirection == ScaleDownDirection {
		scaleDownRule = rule
	}
	return HPABehaviorWithScaleUpAndDownRules(scaleUpRule, scaleDownRule)
}

func HPAScalingRuleWithStabilizationWindow(stabilizationDuration int32) *autoscalingv2.HPAScalingRules {
	return &autoscalingv2.HPAScalingRules{
		StabilizationWindowSeconds: &stabilizationDuration,
	}
}

func HPAScalingRuleWithPolicyDisabled() *autoscalingv2.HPAScalingRules {
	disabledPolicy := autoscalingv2.DisabledPolicySelect
	return &autoscalingv2.HPAScalingRules{
		SelectPolicy: &disabledPolicy,
	}
}

func HPAScalingRuleWithScalingPolicy(policyType autoscalingv2.HPAScalingPolicyType, value, periodSeconds int32) *autoscalingv2.HPAScalingRules {
	stabilizationWindowDisabledDuration := int32(0)
	selectPolicy := autoscalingv2.MaxChangePolicySelect
	return &autoscalingv2.HPAScalingRules{
		Policies: []autoscalingv2.HPAScalingPolicy{
			{
				Type:          policyType,
				Value:         value,
				PeriodSeconds: periodSeconds,
			},
		},
		SelectPolicy:               &selectPolicy,
		StabilizationWindowSeconds: &stabilizationWindowDisabledDuration,
	}
}

func HPABehaviorWithStabilizationWindows(upscaleStabilization, downscaleStabilization time.Duration) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	scaleUpRule := HPAScalingRuleWithStabilizationWindow(int32(upscaleStabilization.Seconds()))
	scaleDownRule := HPAScalingRuleWithStabilizationWindow(int32(downscaleStabilization.Seconds()))
	return HPABehaviorWithScaleUpAndDownRules(scaleUpRule, scaleDownRule)
}

func HPABehaviorWithScaleDisabled(scalingDirection ScalingDirection) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	scalingRule := HPAScalingRuleWithPolicyDisabled()
	return HPABehaviorWithScalingRuleInDirection(scalingDirection, scalingRule)
}

func HPABehaviorWithScaleLimitedByNumberOfPods(scalingDirection ScalingDirection, numberOfPods, periodSeconds int32) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	scalingRule := HPAScalingRuleWithScalingPolicy(autoscalingv2.PodsScalingPolicy, numberOfPods, periodSeconds)
	return HPABehaviorWithScalingRuleInDirection(scalingDirection, scalingRule)
}

func HPABehaviorWithScaleLimitedByPercentage(scalingDirection ScalingDirection, percentage, periodSeconds int32) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	scalingRule := HPAScalingRuleWithScalingPolicy(autoscalingv2.PercentScalingPolicy, percentage, periodSeconds)
	return HPABehaviorWithScalingRuleInDirection(scalingDirection, scalingRule)
}

func DeleteHPAWithBehavior(rc *ResourceConsumer, autoscalerName string) {
	rc.clientSet.AutoscalingV2().HorizontalPodAutoscalers(rc.nsName).Delete(context.TODO(), autoscalerName, metav1.DeleteOptions{})
}

// SidecarStatusType type for sidecar status
type SidecarStatusType bool

const (
	Enable  SidecarStatusType = true
	Disable SidecarStatusType = false
)

// SidecarWorkloadType type of the sidecar
type SidecarWorkloadType string

const (
	Busy SidecarWorkloadType = "Busy"
	Idle SidecarWorkloadType = "Idle"
)
