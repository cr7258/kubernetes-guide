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

package apps

import (
	"context"
	"fmt"
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	batchinternal "k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ejob "k8s.io/kubernetes/test/e2e/framework/job"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2eresource "k8s.io/kubernetes/test/e2e/framework/resource"
	"k8s.io/kubernetes/test/e2e/scheduling"
	"k8s.io/utils/pointer"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var _ = SIGDescribe("Job", func() {
	f := framework.NewDefaultFramework("job")
	parallelism := int32(2)
	completions := int32(4)

	largeParallelism := int32(90)
	largeCompletions := int32(90)

	backoffLimit := int32(6) // default value

	// Simplest case: N pods succeed
	ginkgo.It("should run a job to completion when tasks succeed", func() {
		ginkgo.By("Creating a job")
		job := e2ejob.NewTestJob("succeed", "all-succeed", v1.RestartPolicyNever, parallelism, completions, nil, backoffLimit)
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring job reaches completions")
		err = e2ejob.WaitForJobComplete(f.ClientSet, f.Namespace.Name, job.Name, completions)
		framework.ExpectNoError(err, "failed to ensure job completion in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring pods for job exist")
		pods, err := e2ejob.GetJobPods(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to get pod list for job in namespace: %s", f.Namespace.Name)
		successes := int32(0)
		for _, pod := range pods.Items {
			if pod.Status.Phase == v1.PodSucceeded {
				successes++
			}
		}
		framework.ExpectEqual(successes, completions, "epected %d successful job pods, but got  %d", completions, successes)
	})

	ginkgo.It("should not create pods when created in suspend state", func() {
		ginkgo.By("Creating a job with suspend=true")
		job := e2ejob.NewTestJob("succeed", "suspend-true-to-false", v1.RestartPolicyNever, parallelism, completions, nil, backoffLimit)
		job.Spec.Suspend = pointer.BoolPtr(true)
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring pods aren't created for job")
		framework.ExpectEqual(wait.Poll(framework.Poll, wait.ForeverTestTimeout, func() (bool, error) {
			pods, err := e2ejob.GetJobPods(f.ClientSet, f.Namespace.Name, job.Name)
			if err != nil {
				return false, err
			}
			return len(pods.Items) > 0, nil
		}), wait.ErrWaitTimeout)

		ginkgo.By("Checking Job status to observe Suspended state")
		job, err = e2ejob.GetJob(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to retrieve latest job object")
		exists := false
		for _, c := range job.Status.Conditions {
			if c.Type == batchv1.JobSuspended {
				exists = true
				break
			}
		}
		framework.ExpectEqual(exists, true)

		ginkgo.By("Updating the job with suspend=false")
		job.Spec.Suspend = pointer.BoolPtr(false)
		job, err = e2ejob.UpdateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to update job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Waiting for job to complete")
		err = e2ejob.WaitForJobComplete(f.ClientSet, f.Namespace.Name, job.Name, completions)
		framework.ExpectNoError(err, "failed to ensure job completion in namespace: %s", f.Namespace.Name)
	})

	ginkgo.It("should delete pods when suspended", func() {
		ginkgo.By("Creating a job with suspend=false")
		job := e2ejob.NewTestJob("notTerminate", "suspend-false-to-true", v1.RestartPolicyNever, parallelism, completions, nil, backoffLimit)
		job.Spec.Suspend = pointer.BoolPtr(false)
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensure pods equal to paralellism count is attached to the job")
		err = e2ejob.WaitForAllJobPodsRunning(f.ClientSet, f.Namespace.Name, job.Name, parallelism)
		framework.ExpectNoError(err, "failed to ensure number of pods associated with job %s is equal to parallelism count in namespace: %s", job.Name, f.Namespace.Name)

		ginkgo.By("Updating the job with suspend=true")
		job, err = e2ejob.GetJob(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to retrieve latest job object")
		job.Spec.Suspend = pointer.BoolPtr(true)
		job, err = e2ejob.UpdateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to update job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring pods are deleted")
		err = e2ejob.WaitForAllJobPodsGone(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to ensure pods are deleted after suspend=true")

		ginkgo.By("Checking Job status to observe Suspended state")
		job, err = e2ejob.GetJob(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to retrieve latest job object")
		exists := false
		for _, c := range job.Status.Conditions {
			if c.Type == batchv1.JobSuspended {
				exists = true
				break
			}
		}
		framework.ExpectEqual(exists, true)
	})

	/*
		Testcase: Ensure Pods of an Indexed Job get a unique index.
		Description: Create an Indexed Job, wait for completion, capture the output of the pods and verify that they contain the completion index.
	*/
	ginkgo.It("should create pods for an Indexed job with completion indexes and specified hostname", func() {
		ginkgo.By("Creating Indexed job")
		job := e2ejob.NewTestJob("succeed", "indexed-job", v1.RestartPolicyNever, parallelism, completions, nil, backoffLimit)
		mode := batchv1.IndexedCompletion
		job.Spec.CompletionMode = &mode
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create indexed job in namespace %s", f.Namespace.Name)

		ginkgo.By("Ensuring job reaches completions")
		err = e2ejob.WaitForJobComplete(f.ClientSet, f.Namespace.Name, job.Name, completions)
		framework.ExpectNoError(err, "failed to ensure job completion in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring pods with index for job exist")
		pods, err := e2ejob.GetJobPods(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to get pod list for job in namespace: %s", f.Namespace.Name)
		succeededIndexes := sets.NewInt()
		for _, pod := range pods.Items {
			if pod.Status.Phase == v1.PodSucceeded && pod.Annotations != nil {
				ix, err := strconv.Atoi(pod.Annotations[batchv1.JobCompletionIndexAnnotation])
				framework.ExpectNoError(err, "failed obtaining completion index from pod in namespace: %s", f.Namespace.Name)
				succeededIndexes.Insert(ix)
				expectedName := fmt.Sprintf("%s-%d", job.Name, ix)
				framework.ExpectEqual(pod.Spec.Hostname, expectedName, "expected completed pod with hostname %s, but got %s", expectedName, pod.Spec.Hostname)
			}
		}
		gotIndexes := succeededIndexes.List()
		wantIndexes := []int{0, 1, 2, 3}
		framework.ExpectEqual(gotIndexes, wantIndexes, "expected completed indexes %s, but got %s", wantIndexes, gotIndexes)
	})

	/*
		Testcase: Ensure that the pods associated with the job are removed once the job is deleted
		Description: Create a job and ensure the associated pod count is equal to paralellism count. Delete the
		job and ensure if the pods associated with the job have been removed
	*/
	ginkgo.It("should remove pods when job is deleted", func() {
		ginkgo.By("Creating a job")
		job := e2ejob.NewTestJob("notTerminate", "all-pods-removed", v1.RestartPolicyNever, parallelism, completions, nil, backoffLimit)
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensure pods equal to paralellism count is attached to the job")
		err = e2ejob.WaitForAllJobPodsRunning(f.ClientSet, f.Namespace.Name, job.Name, parallelism)
		framework.ExpectNoError(err, "failed to ensure number of pods associated with job %s is equal to parallelism count in namespace: %s", job.Name, f.Namespace.Name)

		ginkgo.By("Delete the job")
		err = e2eresource.DeleteResourceAndWaitForGC(f.ClientSet, batchinternal.Kind("Job"), f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to delete the job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensure the pods associated with the job are also deleted")
		err = e2ejob.WaitForAllJobPodsGone(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to get PodList for job %s in namespace: %s", job.Name, f.Namespace.Name)
	})

	/*
		Release: v1.16
		Testname: Jobs, completion after task failure
		Description: Explicitly cause the tasks to fail once initially. After restarting, the Job MUST
		execute to completion.
	*/
	framework.ConformanceIt("should run a job to completion when tasks sometimes fail and are locally restarted", func() {
		ginkgo.By("Creating a job")
		// One failure, then a success, local restarts.
		// We can't use the random failure approach, because kubelet will
		// throttle frequently failing containers in a given pod, ramping
		// up to 5 minutes between restarts, making test timeout due to
		// successive failures too likely with a reasonable test timeout.
		job := e2ejob.NewTestJob("failOnce", "fail-once-local", v1.RestartPolicyOnFailure, parallelism, completions, nil, backoffLimit)
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring job reaches completions")
		err = e2ejob.WaitForJobComplete(f.ClientSet, f.Namespace.Name, job.Name, completions)
		framework.ExpectNoError(err, "failed to ensure job completion in namespace: %s", f.Namespace.Name)
	})

	// Pods sometimes fail, but eventually succeed, after pod restarts
	ginkgo.It("should run a job to completion when tasks sometimes fail and are not locally restarted", func() {
		// One failure, then a success, no local restarts.
		// We can't use the random failure approach, because JobController
		// will throttle frequently failing Pods of a given Job, ramping
		// up to 6 minutes between restarts, making test timeout due to
		// successive failures.
		// Instead, we force the Job's Pods to be scheduled to a single Node
		// and use a hostPath volume to persist data across new Pods.
		ginkgo.By("Looking for a node to schedule job pod")
		node, err := e2enode.GetRandomReadySchedulableNode(f.ClientSet)
		framework.ExpectNoError(err)

		ginkgo.By("Creating a job")
		job := e2ejob.NewTestJobOnNode("failOnce", "fail-once-non-local", v1.RestartPolicyNever, parallelism, completions, nil, backoffLimit, node.Name)
		job, err = e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring job reaches completions")
		err = e2ejob.WaitForJobComplete(f.ClientSet, f.Namespace.Name, job.Name, *job.Spec.Completions)
		framework.ExpectNoError(err, "failed to ensure job completion in namespace: %s", f.Namespace.Name)
	})

	ginkgo.It("should fail when exceeds active deadline", func() {
		ginkgo.By("Creating a job")
		var activeDeadlineSeconds int64 = 1
		job := e2ejob.NewTestJob("notTerminate", "exceed-active-deadline", v1.RestartPolicyNever, parallelism, completions, &activeDeadlineSeconds, backoffLimit)
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)
		ginkgo.By("Ensuring job past active deadline")
		err = waitForJobFailure(f.ClientSet, f.Namespace.Name, job.Name, time.Duration(activeDeadlineSeconds+15)*time.Second, "DeadlineExceeded")
		framework.ExpectNoError(err, "failed to ensure job past active deadline in namespace: %s", f.Namespace.Name)
	})

	/*
		Release: v1.15
		Testname: Jobs, active pods, graceful termination
		Description: Create a job. Ensure the active pods reflect paralellism in the namespace and delete the job. Job MUST be deleted successfully.
	*/
	framework.ConformanceIt("should delete a job", func() {
		ginkgo.By("Creating a job")
		job := e2ejob.NewTestJob("notTerminate", "foo", v1.RestartPolicyNever, parallelism, completions, nil, backoffLimit)
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring active pods == parallelism")
		err = e2ejob.WaitForAllJobPodsRunning(f.ClientSet, f.Namespace.Name, job.Name, parallelism)
		framework.ExpectNoError(err, "failed to ensure active pods == parallelism in namespace: %s", f.Namespace.Name)

		ginkgo.By("delete a job")
		framework.ExpectNoError(e2eresource.DeleteResourceAndWaitForGC(f.ClientSet, batchinternal.Kind("Job"), f.Namespace.Name, job.Name))

		ginkgo.By("Ensuring job was deleted")
		_, err = e2ejob.GetJob(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectError(err, "failed to ensure job %s was deleted in namespace: %s", job.Name, f.Namespace.Name)
		framework.ExpectEqual(apierrors.IsNotFound(err), true)
	})

	/*
		Release: v1.16
		Testname: Jobs, orphan pods, re-adoption
		Description: Create a parallel job. The number of Pods MUST equal the level of parallelism.
		Orphan a Pod by modifying its owner reference. The Job MUST re-adopt the orphan pod.
		Modify the labels of one of the Job's Pods. The Job MUST release the Pod.
	*/
	framework.ConformanceIt("should adopt matching orphans and release non-matching pods", func() {
		ginkgo.By("Creating a job")
		job := e2ejob.NewTestJob("notTerminate", "adopt-release", v1.RestartPolicyNever, parallelism, completions, nil, backoffLimit)
		// Replace job with the one returned from Create() so it has the UID.
		// Save Kind since it won't be populated in the returned job.
		kind := job.Kind
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)
		job.Kind = kind

		ginkgo.By("Ensuring active pods == parallelism")
		err = e2ejob.WaitForAllJobPodsRunning(f.ClientSet, f.Namespace.Name, job.Name, parallelism)
		framework.ExpectNoError(err, "failed to ensure active pods == parallelism in namespace: %s", f.Namespace.Name)

		ginkgo.By("Orphaning one of the Job's Pods")
		pods, err := e2ejob.GetJobPods(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to get PodList for job %s in namespace: %s", job.Name, f.Namespace.Name)
		gomega.Expect(pods.Items).To(gomega.HaveLen(int(parallelism)))
		pod := pods.Items[0]
		f.PodClient().Update(pod.Name, func(pod *v1.Pod) {
			pod.OwnerReferences = nil
		})

		ginkgo.By("Checking that the Job readopts the Pod")
		gomega.Expect(e2epod.WaitForPodCondition(f.ClientSet, pod.Namespace, pod.Name, "adopted", e2ejob.JobTimeout,
			func(pod *v1.Pod) (bool, error) {
				controllerRef := metav1.GetControllerOf(pod)
				if controllerRef == nil {
					return false, nil
				}
				if controllerRef.Kind != job.Kind || controllerRef.Name != job.Name || controllerRef.UID != job.UID {
					return false, fmt.Errorf("pod has wrong controllerRef: got %v, want %v", controllerRef, job)
				}
				return true, nil
			},
		)).To(gomega.Succeed(), "wait for pod %q to be readopted", pod.Name)

		ginkgo.By("Removing the labels from the Job's Pod")
		f.PodClient().Update(pod.Name, func(pod *v1.Pod) {
			pod.Labels = nil
		})

		ginkgo.By("Checking that the Job releases the Pod")
		gomega.Expect(e2epod.WaitForPodCondition(f.ClientSet, pod.Namespace, pod.Name, "released", e2ejob.JobTimeout,
			func(pod *v1.Pod) (bool, error) {
				controllerRef := metav1.GetControllerOf(pod)
				if controllerRef != nil {
					return false, nil
				}
				return true, nil
			},
		)).To(gomega.Succeed(), "wait for pod %q to be released", pod.Name)
	})

	ginkgo.It("should fail to exceed backoffLimit", func() {
		ginkgo.By("Creating a job")
		backoff := 1
		job := e2ejob.NewTestJob("fail", "backofflimit", v1.RestartPolicyNever, 1, 1, nil, int32(backoff))
		job, err := e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)
		ginkgo.By("Ensuring job exceed backofflimit")

		err = waitForJobFailure(f.ClientSet, f.Namespace.Name, job.Name, e2ejob.JobTimeout, "BackoffLimitExceeded")
		framework.ExpectNoError(err, "failed to ensure job exceed backofflimit in namespace: %s", f.Namespace.Name)

		ginkgo.By(fmt.Sprintf("Checking that %d pod created and status is failed", backoff+1))
		pods, err := e2ejob.GetJobPods(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to get PodList for job %s in namespace: %s", job.Name, f.Namespace.Name)
		gomega.Expect(pods.Items).To(gomega.HaveLen(backoff + 1))
		for _, pod := range pods.Items {
			framework.ExpectEqual(pod.Status.Phase, v1.PodFailed)
		}
	})

	ginkgo.It("should run a job to completion with CPU requests [Serial]", func() {
		ginkgo.By("Creating a job that with CPU requests")

		testNodeName := scheduling.GetNodeThatCanRunPod(f)
		targetNode, err := f.ClientSet.CoreV1().Nodes().Get(context.TODO(), testNodeName, metav1.GetOptions{})
		framework.ExpectNoError(err, "unable to get node object for node %v", testNodeName)

		cpu, ok := targetNode.Status.Allocatable[v1.ResourceCPU]
		if !ok {
			framework.Failf("Unable to get node's %q cpu", targetNode.Name)
		}

		cpuRequest := fmt.Sprint(int64(0.2 * float64(cpu.Value())))

		backoff := 0
		ginkgo.By("Creating a job")
		job := e2ejob.NewTestJob("succeed", "all-succeed", v1.RestartPolicyNever, largeParallelism, largeCompletions, nil, int32(backoff))
		for i := range job.Spec.Template.Spec.Containers {
			job.Spec.Template.Spec.Containers[i].Resources = v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceCPU: resource.MustParse(cpuRequest),
				},
			}
			job.Spec.Template.Spec.NodeSelector = map[string]string{"kubernetes.io/hostname": testNodeName}
		}

		framework.Logf("Creating job %q with a node hostname selector %q wth cpu request %q", job.Name, testNodeName, cpuRequest)
		job, err = e2ejob.CreateJob(f.ClientSet, f.Namespace.Name, job)
		framework.ExpectNoError(err, "failed to create job in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring job reaches completions")
		err = e2ejob.WaitForJobComplete(f.ClientSet, f.Namespace.Name, job.Name, largeCompletions)
		framework.ExpectNoError(err, "failed to ensure job completion in namespace: %s", f.Namespace.Name)

		ginkgo.By("Ensuring pods for job exist")
		pods, err := e2ejob.GetJobPods(f.ClientSet, f.Namespace.Name, job.Name)
		framework.ExpectNoError(err, "failed to get pod list for job in namespace: %s", f.Namespace.Name)
		successes := int32(0)
		for _, pod := range pods.Items {
			if pod.Status.Phase == v1.PodSucceeded {
				successes++
			}
		}
		framework.ExpectEqual(successes, largeCompletions, "expected %d successful job pods, but got  %d", largeCompletions, successes)
	})
})

// waitForJobFailure uses c to wait for up to timeout for the Job named jobName in namespace ns to fail.
func waitForJobFailure(c clientset.Interface, ns, jobName string, timeout time.Duration, reason string) error {
	return wait.Poll(framework.Poll, timeout, func() (bool, error) {
		curr, err := c.BatchV1().Jobs(ns).Get(context.TODO(), jobName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, c := range curr.Status.Conditions {
			if c.Type == batchv1.JobFailed && c.Status == v1.ConditionTrue {
				if reason == "" || reason == c.Reason {
					return true, nil
				}
			}
		}
		return false, nil
	})
}
