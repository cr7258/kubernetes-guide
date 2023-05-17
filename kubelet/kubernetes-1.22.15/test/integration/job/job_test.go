/*
Copyright 2021 The Kubernetes Authors.

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

package job

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	jobcontroller "k8s.io/kubernetes/pkg/controller/job"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/test/integration/framework"
	"k8s.io/utils/pointer"
)

const waitInterval = 500 * time.Millisecond

// TestNonParallelJob tests that a Job that only executes one Pod. The test
// recreates the Job controller at some points to make sure a new controller
// is able to pickup.
func TestNonParallelJob(t *testing.T) {
	for _, wFinalizers := range []bool{false, true} {
		t.Run(fmt.Sprintf("finalizers=%t", wFinalizers), func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, wFinalizers)()

			closeFn, restConfig, clientSet, ns := setup(t, "simple")
			defer closeFn()
			ctx, cancel := startJobController(restConfig, clientSet)
			defer func() {
				cancel()
			}()

			jobObj, err := createJobWithDefaults(ctx, clientSet, ns.Name, &batchv1.Job{})
			if err != nil {
				t.Fatalf("Failed to create Job: %v", err)
			}
			if got := hasJobTrackingAnnotation(jobObj); got != wFinalizers {
				t.Errorf("apiserver created job with tracking annotation: %t, want %t", got, wFinalizers)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active: 1,
			}, wFinalizers)

			// Restarting controller.
			cancel()
			ctx, cancel = startJobController(restConfig, clientSet)

			// Failed Pod is replaced.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodFailed, 1); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pod: %v", v1.PodFailed, err)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active: 1,
				Failed: 1,
			}, wFinalizers)

			// Restarting controller.
			cancel()
			ctx, cancel = startJobController(restConfig, clientSet)

			// No more Pods are created after the Pod succeeds.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodSucceeded, 1); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pod: %v", v1.PodSucceeded, err)
			}
			validateJobSucceeded(ctx, t, clientSet, jobObj)
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Failed:    1,
				Succeeded: 1,
			}, false)
			validateFinishedPodsNoFinalizer(ctx, t, clientSet, jobObj)
		})
	}
}

func TestParallelJob(t *testing.T) {
	for _, wFinalizers := range []bool{false, true} {
		t.Run(fmt.Sprintf("finalizers=%t", wFinalizers), func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, wFinalizers)()

			closeFn, restConfig, clientSet, ns := setup(t, "parallel")
			defer closeFn()
			ctx, cancel := startJobController(restConfig, clientSet)
			defer cancel()

			jobObj, err := createJobWithDefaults(ctx, clientSet, ns.Name, &batchv1.Job{
				Spec: batchv1.JobSpec{
					Parallelism: pointer.Int32Ptr(5),
				},
			})
			if err != nil {
				t.Fatalf("Failed to create Job: %v", err)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active: 5,
			}, wFinalizers)
			// Failed Pods are replaced.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodFailed, 2); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pods: %v", v1.PodFailed, err)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active: 5,
				Failed: 2,
			}, wFinalizers)
			// Once one Pod succeeds, no more Pods are created, even if some fail.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodSucceeded, 1); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pod: %v", v1.PodSucceeded, err)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Failed:    2,
				Succeeded: 1,
				Active:    4,
			}, wFinalizers)
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodFailed, 2); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pods: %v", v1.PodFailed, err)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Failed:    4,
				Succeeded: 1,
				Active:    2,
			}, wFinalizers)
			// No more Pods are created after remaining Pods succeed.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodSucceeded, 2); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pods: %v", v1.PodSucceeded, err)
			}
			validateJobSucceeded(ctx, t, clientSet, jobObj)
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Failed:    4,
				Succeeded: 3,
			}, false)
			validateFinishedPodsNoFinalizer(ctx, t, clientSet, jobObj)
		})
	}
}

func TestParallelJobWithCompletions(t *testing.T) {
	for _, wFinalizers := range []bool{false, true} {
		t.Run(fmt.Sprintf("finalizers=%t", wFinalizers), func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, wFinalizers)()
			closeFn, restConfig, clientSet, ns := setup(t, "completions")
			defer closeFn()
			ctx, cancel := startJobController(restConfig, clientSet)
			defer cancel()

			jobObj, err := createJobWithDefaults(ctx, clientSet, ns.Name, &batchv1.Job{
				Spec: batchv1.JobSpec{
					Parallelism: pointer.Int32Ptr(4),
					Completions: pointer.Int32Ptr(6),
				},
			})
			if err != nil {
				t.Fatalf("Failed to create Job: %v", err)
			}
			if got := hasJobTrackingAnnotation(jobObj); got != wFinalizers {
				t.Errorf("apiserver created job with tracking annotation: %t, want %t", got, wFinalizers)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active: 4,
			}, wFinalizers)
			// Failed Pods are replaced.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodFailed, 2); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pods: %v", v1.PodFailed, err)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active: 4,
				Failed: 2,
			}, wFinalizers)
			// Pods are created until the number of succeeded Pods equals completions.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodSucceeded, 3); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pod: %v", v1.PodSucceeded, err)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Failed:    2,
				Succeeded: 3,
				Active:    3,
			}, wFinalizers)
			// No more Pods are created after the Job completes.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodSucceeded, 3); err != nil {
				t.Fatalf("Failed setting phase %s on Job Pods: %v", v1.PodSucceeded, err)
			}
			validateJobSucceeded(ctx, t, clientSet, jobObj)
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Failed:    2,
				Succeeded: 6,
			}, false)
			validateFinishedPodsNoFinalizer(ctx, t, clientSet, jobObj)
		})
	}
}

func TestIndexedJob(t *testing.T) {
	for _, wFinalizers := range []bool{false, true} {
		t.Run(fmt.Sprintf("finalizers=%t", wFinalizers), func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, wFinalizers)()
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.IndexedJob, true)()

			closeFn, restConfig, clientSet, ns := setup(t, "indexed")
			defer closeFn()
			ctx, cancel := startJobController(restConfig, clientSet)
			defer func() {
				cancel()
			}()

			mode := batchv1.IndexedCompletion
			jobObj, err := createJobWithDefaults(ctx, clientSet, ns.Name, &batchv1.Job{
				Spec: batchv1.JobSpec{
					Parallelism:    pointer.Int32Ptr(3),
					Completions:    pointer.Int32Ptr(4),
					CompletionMode: &mode,
				},
			})
			if err != nil {
				t.Fatalf("Failed to create Job: %v", err)
			}
			if got := hasJobTrackingAnnotation(jobObj); got != wFinalizers {
				t.Errorf("apiserver created job with tracking annotation: %t, want %t", got, wFinalizers)
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active: 3,
			}, wFinalizers)
			validateIndexedJobPods(ctx, t, clientSet, jobObj, sets.NewInt(0, 1, 2), "")

			// One Pod succeeds.
			if err := setJobPhaseForIndex(ctx, clientSet, jobObj, v1.PodSucceeded, 1); err != nil {
				t.Fatal("Failed trying to succeed pod with index 1")
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active:    3,
				Succeeded: 1,
			}, wFinalizers)
			validateIndexedJobPods(ctx, t, clientSet, jobObj, sets.NewInt(0, 2, 3), "1")

			// Disable feature gate and restart controller.
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.IndexedJob, false)()
			cancel()
			ctx, cancel = startJobController(restConfig, clientSet)
			events, err := clientSet.EventsV1().Events(ns.Name).Watch(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer events.Stop()

			// One Pod fails, but no recreations happen because feature is disabled.
			if err := setJobPhaseForIndex(ctx, clientSet, jobObj, v1.PodFailed, 2); err != nil {
				t.Fatal("Failed trying to succeed pod with index 2")
			}
			if err := waitForEvent(events, jobObj.UID, "IndexedJobDisabled"); err != nil {
				t.Errorf("Waiting for an event for IndexedJobDisabled: %v", err)
			}
			validateIndexedJobPods(ctx, t, clientSet, jobObj, sets.NewInt(0, 3), "1")

			// Re-enable feature gate and restart controller. Failed Pod should be recreated now.
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.IndexedJob, true)()
			cancel()
			ctx, cancel = startJobController(restConfig, clientSet)

			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active:    3,
				Failed:    1,
				Succeeded: 1,
			}, wFinalizers)
			validateIndexedJobPods(ctx, t, clientSet, jobObj, sets.NewInt(0, 2, 3), "1")

			// Remaining Pods succeed.
			if err := setJobPodsPhase(ctx, clientSet, jobObj, v1.PodSucceeded, 3); err != nil {
				t.Fatal("Failed trying to succeed remaining pods")
			}
			validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
				Active:    0,
				Failed:    1,
				Succeeded: 4,
			}, false)
			validateIndexedJobPods(ctx, t, clientSet, jobObj, nil, "0-3")
			validateJobSucceeded(ctx, t, clientSet, jobObj)
			validateFinishedPodsNoFinalizer(ctx, t, clientSet, jobObj)
		})
	}
}

// TestDisableJobTrackingWithFinalizers ensures that when the
// JobTrackingWithFinalizers feature is disabled, tracking finalizers are
// removed from all pods, but Job continues to be tracked.
// This test can be removed once the feature graduates to GA.
func TestDisableJobTrackingWithFinalizers(t *testing.T) {
	// Step 1: job created while feature is enabled.
	defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, true)()

	closeFn, restConfig, clientSet, ns := setup(t, "simple")
	defer closeFn()
	ctx, cancel := startJobController(restConfig, clientSet)
	defer func() {
		cancel()
	}()

	jobObj, err := createJobWithDefaults(ctx, clientSet, ns.Name, &batchv1.Job{
		Spec: batchv1.JobSpec{
			Parallelism: pointer.Int32Ptr(2),
		},
	})
	if err != nil {
		t.Fatalf("Failed to create Job: %v", err)
	}
	if !hasJobTrackingAnnotation(jobObj) {
		t.Error("apiserver didn't add the tracking annotation")
	}
	validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
		Active: 2,
	}, true)

	// Step 2: Disable tracking with finalizers.
	defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, false)()
	cancel()

	// Fail a pod while Job controller is stopped.
	if err := setJobPodsPhase(context.Background(), clientSet, jobObj, v1.PodFailed, 1); err != nil {
		t.Fatalf("Failed setting phase %s on Job Pod: %v", v1.PodFailed, err)
	}

	// Restart controller.
	ctx, cancel = startJobController(restConfig, clientSet)

	// Ensure Job continues to be tracked and finalizers are removed.
	validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
		Active: 2,
		Failed: 1,
	}, false)

	jobObj, err = clientSet.BatchV1().Jobs(jobObj.Namespace).Get(ctx, jobObj.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Obtaining updated Job object: %v", err)
	}
	if hasJobTrackingAnnotation(jobObj) {
		t.Error("controller didn't remove the tracking annotation")
	}

	// Step 3: Reenable tracking with finalizers.
	defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, true)()
	cancel()

	// Succeed a pod while Job controller is stopped.
	if err := setJobPodsPhase(context.Background(), clientSet, jobObj, v1.PodSucceeded, 1); err != nil {
		t.Fatalf("Failed setting phase %s on Job Pod: %v", v1.PodFailed, err)
	}

	// Restart controller.
	ctx, cancel = startJobController(restConfig, clientSet)

	// Ensure Job continues to be tracked and finalizers are removed.
	validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
		Active:    1,
		Failed:    1,
		Succeeded: 1,
	}, false)
}

func TestOrphanPodsFinalizersCleared(t *testing.T) {
	// Step 0: job created while feature is enabled.
	defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, true)()

	closeFn, restConfig, clientSet, ns := setup(t, "simple")
	defer closeFn()
	ctx, cancel := startJobController(restConfig, clientSet)
	defer func() {
		cancel()
	}()

	jobObj, err := createJobWithDefaults(ctx, clientSet, ns.Name, &batchv1.Job{
		Spec: batchv1.JobSpec{
			Parallelism: pointer.Int32Ptr(1),
		},
	})
	if err != nil {
		t.Fatalf("Failed to create Job: %v", err)
	}
	if !hasJobTrackingAnnotation(jobObj) {
		t.Error("apiserver didn't add the tracking annotation")
	}
	validateJobPodsStatus(ctx, t, clientSet, jobObj, podsByStatus{
		Active: 1,
	}, true)

	// Step 2: Disable tracking with finalizers.
	defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.JobTrackingWithFinalizers, false)()
	cancel()

	// Delete the Job while controller is stopped.
	err = clientSet.BatchV1().Jobs(jobObj.Namespace).Delete(context.Background(), jobObj.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Failed to delete job: %v", err)
	}

	// Restart controller.
	ctx, cancel = startJobController(restConfig, clientSet)
	if err := wait.Poll(waitInterval, wait.ForeverTestTimeout, func() (done bool, err error) {
		pods, err := clientSet.CoreV1().Pods(jobObj.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Falied to list Job Pods: %v", err)
		}
		sawPods := false
		for _, pod := range pods.Items {
			if isPodOwnedByJob(&pod, jobObj) {
				if hasJobTrackingFinalizer(&pod) {
					return false, nil
				}
				sawPods = true
			}
		}
		return sawPods, nil
	}); err != nil {
		t.Errorf("Waiting for finalizers to be removed: %v", err)
	}
}

func TestSuspendJob(t *testing.T) {
	type step struct {
		flag       bool
		wantActive int
		wantStatus v1.ConditionStatus
		wantReason string
	}
	testCases := []struct {
		featureGate bool
		create      step
		update      step
	}{
		// Exhaustively test all combinations other than trivial true->true and
		// false->false cases.
		{
			featureGate: true,
			create:      step{flag: false, wantActive: 2},
			update:      step{flag: true, wantActive: 0, wantStatus: v1.ConditionTrue, wantReason: "Suspended"},
		},
		{
			featureGate: true,
			create:      step{flag: true, wantActive: 0, wantStatus: v1.ConditionTrue, wantReason: "Suspended"},
			update:      step{flag: false, wantActive: 2, wantStatus: v1.ConditionFalse, wantReason: "Resumed"},
		},
		{
			featureGate: false,
			create:      step{flag: false, wantActive: 2},
			update:      step{flag: true, wantActive: 2},
		},
		{
			featureGate: false,
			create:      step{flag: true, wantActive: 2},
			update:      step{flag: false, wantActive: 2},
		},
	}

	for _, tc := range testCases {
		name := fmt.Sprintf("feature=%v,create=%v,update=%v", tc.featureGate, tc.create.flag, tc.update.flag)
		t.Run(name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.SuspendJob, tc.featureGate)()

			closeFn, restConfig, clientSet, ns := setup(t, "suspend")
			defer closeFn()
			ctx, cancel := startJobController(restConfig, clientSet)
			defer cancel()
			events, err := clientSet.EventsV1().Events(ns.Name).Watch(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer events.Stop()

			parallelism := int32(2)
			job, err := createJobWithDefaults(ctx, clientSet, ns.Name, &batchv1.Job{
				Spec: batchv1.JobSpec{
					Parallelism: pointer.Int32Ptr(parallelism),
					Completions: pointer.Int32Ptr(4),
					Suspend:     pointer.BoolPtr(tc.create.flag),
				},
			})
			if err != nil {
				t.Fatalf("Failed to create Job: %v", err)
			}

			validate := func(s string, active int, status v1.ConditionStatus, reason string) {
				validateJobPodsStatus(ctx, t, clientSet, job, podsByStatus{
					Active: active,
				}, false)
				job, err = clientSet.BatchV1().Jobs(ns.Name).Get(ctx, job.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("Failed to get Job after %s: %v", s, err)
				}
				if got, want := getJobConditionStatus(ctx, job, batchv1.JobSuspended), status; got != want {
					t.Errorf("Unexpected Job condition %q status after %s: got %q, want %q", batchv1.JobSuspended, s, got, want)
				}
				if err := waitForEvent(events, job.UID, reason); err != nil {
					t.Errorf("Waiting for event with reason %q after %s: %v", reason, s, err)
				}
			}
			validate("create", tc.create.wantActive, tc.create.wantStatus, tc.create.wantReason)

			job.Spec.Suspend = pointer.BoolPtr(tc.update.flag)
			job, err = clientSet.BatchV1().Jobs(ns.Name).Update(ctx, job, metav1.UpdateOptions{})
			if err != nil {
				t.Fatalf("Failed to update Job: %v", err)
			}
			validate("update", tc.update.wantActive, tc.update.wantStatus, tc.update.wantReason)
		})
	}
}

func TestSuspendJobControllerRestart(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.SuspendJob, true)()

	closeFn, restConfig, clientSet, ns := setup(t, "suspend")
	defer closeFn()
	ctx, cancel := startJobController(restConfig, clientSet)
	defer func() {
		cancel()
	}()

	job, err := createJobWithDefaults(ctx, clientSet, ns.Name, &batchv1.Job{
		Spec: batchv1.JobSpec{
			Parallelism: pointer.Int32Ptr(2),
			Completions: pointer.Int32Ptr(4),
			Suspend:     pointer.BoolPtr(true),
		},
	})
	if err != nil {
		t.Fatalf("Failed to create Job: %v", err)
	}
	validateJobPodsStatus(ctx, t, clientSet, job, podsByStatus{
		Active: 0,
	}, false)

	// Disable feature gate and restart controller to test that pods get created.
	defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.SuspendJob, false)()
	cancel()
	ctx, cancel = startJobController(restConfig, clientSet)
	job, err = clientSet.BatchV1().Jobs(ns.Name).Get(ctx, job.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get Job: %v", err)
	}
	validateJobPodsStatus(ctx, t, clientSet, job, podsByStatus{
		Active: 2,
	}, false)
}

type podsByStatus struct {
	Active    int
	Failed    int
	Succeeded int
}

func validateJobPodsStatus(ctx context.Context, t *testing.T, clientSet clientset.Interface, jobObj *batchv1.Job, desired podsByStatus, wFinalizer bool) {
	t.Helper()
	var actualCounts podsByStatus
	if err := wait.Poll(waitInterval, wait.ForeverTestTimeout, func() (bool, error) {
		updatedJob, err := clientSet.BatchV1().Jobs(jobObj.Namespace).Get(ctx, jobObj.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get updated Job: %v", err)
		}
		actualCounts = podsByStatus{
			Active:    int(updatedJob.Status.Active),
			Succeeded: int(updatedJob.Status.Succeeded),
			Failed:    int(updatedJob.Status.Failed),
		}
		return cmp.Equal(actualCounts, desired), nil
	}); err != nil {
		diff := cmp.Diff(desired, actualCounts)
		t.Errorf("Waiting for Job Status: %v\nPods (-want,+got):\n%s", err, diff)
	}
	var active []*v1.Pod
	if err := wait.PollImmediate(waitInterval, wait.ForeverTestTimeout, func() (bool, error) {
		pods, err := clientSet.CoreV1().Pods(jobObj.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Failed to list Job Pods: %v", err)
		}
		active = nil
		for _, pod := range pods.Items {
			phase := pod.Status.Phase
			if isPodOwnedByJob(&pod, jobObj) && (phase == v1.PodPending || phase == v1.PodRunning) {
				p := pod
				active = append(active, &p)
			}
		}
		return len(active) == desired.Active, nil
	}); err != nil {
		if len(active) != desired.Active {
			t.Errorf("Found %d active Pods, want %d", len(active), desired.Active)
		}
	}
	for _, p := range active {
		if got := hasJobTrackingFinalizer(p); got != wFinalizer {
			t.Errorf("Pod %s has tracking finalizer %t, want %t", p.Name, got, wFinalizer)
		}
	}
}

func validateFinishedPodsNoFinalizer(ctx context.Context, t *testing.T, clientSet clientset.Interface, jobObj *batchv1.Job) {
	t.Helper()
	pods, err := clientSet.CoreV1().Pods(jobObj.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list Job Pods: %v", err)
	}
	for _, pod := range pods.Items {
		phase := pod.Status.Phase
		if isPodOwnedByJob(&pod, jobObj) && (phase == v1.PodPending || phase == v1.PodRunning) && hasJobTrackingFinalizer(&pod) {
			t.Errorf("Finished pod %s still has a tracking finalizer", pod.Name)
		}
	}
}

// validateIndexedJobPods validates indexes and hostname of
// active and completed Pods of an Indexed Job.
// Call after validateJobPodsStatus
func validateIndexedJobPods(ctx context.Context, t *testing.T, clientSet clientset.Interface, jobObj *batchv1.Job, wantActive sets.Int, gotCompleted string) {
	t.Helper()
	updatedJob, err := clientSet.BatchV1().Jobs(jobObj.Namespace).Get(ctx, jobObj.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get updated Job: %v", err)
	}
	if updatedJob.Status.CompletedIndexes != gotCompleted {
		t.Errorf("Got completed indexes %q, want %q", updatedJob.Status.CompletedIndexes, gotCompleted)
	}
	pods, err := clientSet.CoreV1().Pods(jobObj.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list Job Pods: %v", err)
	}
	gotActive := sets.NewInt()
	for _, pod := range pods.Items {
		if isPodOwnedByJob(&pod, jobObj) {
			if pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning {
				ix, err := getCompletionIndex(&pod)
				if err != nil {
					t.Errorf("Failed getting completion index for pod %s: %v", pod.Name, err)
				} else {
					gotActive.Insert(ix)
				}
				expectedName := fmt.Sprintf("%s-%d", jobObj.Name, ix)
				if diff := cmp.Equal(expectedName, pod.Spec.Hostname); !diff {
					t.Errorf("Got pod hostname %s, want %s", pod.Spec.Hostname, expectedName)
				}
			}
		}
	}
	if wantActive == nil {
		wantActive = sets.NewInt()
	}
	if diff := cmp.Diff(wantActive.List(), gotActive.List()); diff != "" {
		t.Errorf("Unexpected active indexes (-want,+got):\n%s", diff)
	}
}

func waitForEvent(events watch.Interface, uid types.UID, reason string) error {
	if reason == "" {
		return nil
	}
	return wait.Poll(waitInterval, wait.ForeverTestTimeout, func() (bool, error) {
		for {
			var ev watch.Event
			select {
			case ev = <-events.ResultChan():
			default:
				return false, nil
			}
			e, ok := ev.Object.(*eventsv1.Event)
			if !ok {
				continue
			}
			ctrl := "job-controller"
			if (e.ReportingController == ctrl || e.DeprecatedSource.Component == ctrl) && e.Reason == reason && e.Regarding.UID == uid {
				return true, nil
			}
		}
	})
}

func getJobConditionStatus(ctx context.Context, job *batchv1.Job, cType batchv1.JobConditionType) v1.ConditionStatus {
	for _, cond := range job.Status.Conditions {
		if cond.Type == cType {
			return cond.Status
		}
	}
	return ""
}

func validateJobSucceeded(ctx context.Context, t *testing.T, clientSet clientset.Interface, jobObj *batchv1.Job) {
	t.Helper()
	if err := wait.Poll(waitInterval, wait.ForeverTestTimeout, func() (bool, error) {
		j, err := clientSet.BatchV1().Jobs(jobObj.Namespace).Get(ctx, jobObj.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to obtain updated Job: %v", err)
		}
		return getJobConditionStatus(ctx, j, batchv1.JobComplete) == v1.ConditionTrue, nil
	}); err != nil {
		t.Errorf("Waiting for Job to succeed: %v", err)
	}
}

func setJobPodsPhase(ctx context.Context, clientSet clientset.Interface, jobObj *batchv1.Job, phase v1.PodPhase, cnt int) error {
	pods, err := clientSet.CoreV1().Pods(jobObj.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing Job Pods: %w", err)
	}
	for _, pod := range pods.Items {
		if cnt == 0 {
			break
		}
		if p := pod.Status.Phase; isPodOwnedByJob(&pod, jobObj) && p != v1.PodFailed && p != v1.PodSucceeded {
			pod.Status.Phase = phase
			_, err := clientSet.CoreV1().Pods(pod.Namespace).UpdateStatus(ctx, &pod, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("updating Pod status: %w", err)
			}
			cnt--
		}
	}
	if cnt != 0 {
		return fmt.Errorf("couldn't set phase on %d Job Pods", cnt)
	}
	return nil
}

func setJobPhaseForIndex(ctx context.Context, clientSet clientset.Interface, jobObj *batchv1.Job, phase v1.PodPhase, ix int) error {
	pods, err := clientSet.CoreV1().Pods(jobObj.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing Job Pods: %w", err)
	}
	for _, pod := range pods.Items {
		if p := pod.Status.Phase; !isPodOwnedByJob(&pod, jobObj) || p == v1.PodFailed || p == v1.PodSucceeded {
			continue
		}
		if pix, err := getCompletionIndex(&pod); err == nil && pix == ix {
			pod.Status.Phase = phase
			_, err := clientSet.CoreV1().Pods(pod.Namespace).UpdateStatus(ctx, &pod, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("updating pod %s status: %w", pod.Name, err)
			}
			return nil
		}
	}
	return errors.New("no pod matching index found")
}

func getCompletionIndex(p *v1.Pod) (int, error) {
	if p.Annotations == nil {
		return 0, errors.New("no annotations found")
	}
	v, ok := p.Annotations[batchv1.JobCompletionIndexAnnotation]
	if !ok {
		return 0, fmt.Errorf("annotation %s not found", batchv1.JobCompletionIndexAnnotation)
	}
	return strconv.Atoi(v)
}

func isPodOwnedByJob(p *v1.Pod, j *batchv1.Job) bool {
	for _, owner := range p.ObjectMeta.OwnerReferences {
		if owner.Kind == "Job" && owner.UID == j.UID {
			return true
		}
	}
	return false
}

func createJobWithDefaults(ctx context.Context, clientSet clientset.Interface, ns string, jobObj *batchv1.Job) (*batchv1.Job, error) {
	if jobObj.Name == "" {
		jobObj.Name = "test-job"
	}
	if len(jobObj.Spec.Template.Spec.Containers) == 0 {
		jobObj.Spec.Template.Spec.Containers = []v1.Container{
			{Name: "foo", Image: "bar"},
		}
	}
	if jobObj.Spec.Template.Spec.RestartPolicy == "" {
		jobObj.Spec.Template.Spec.RestartPolicy = v1.RestartPolicyNever
	}
	return clientSet.BatchV1().Jobs(ns).Create(ctx, jobObj, metav1.CreateOptions{})
}

func setup(t *testing.T, nsBaseName string) (framework.CloseFunc, *restclient.Config, clientset.Interface, *v1.Namespace) {
	controlPlaneConfig := framework.NewIntegrationTestControlPlaneConfig()
	_, server, apiServerCloseFn := framework.RunAnAPIServer(controlPlaneConfig)

	config := restclient.Config{Host: server.URL}
	clientSet, err := clientset.NewForConfig(&config)
	if err != nil {
		t.Fatalf("Error creating clientset: %v", err)
	}
	ns := framework.CreateTestingNamespace(nsBaseName, server, t)
	closeFn := func() {
		framework.DeleteTestingNamespace(ns, server, t)
		apiServerCloseFn()
	}
	return closeFn, &config, clientSet, ns
}

func startJobController(restConfig *restclient.Config, clientSet clientset.Interface) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	resyncPeriod := 12 * time.Hour
	informerSet := informers.NewSharedInformerFactory(clientset.NewForConfigOrDie(restclient.AddUserAgent(restConfig, "cronjob-informers")), resyncPeriod)
	jc := jobcontroller.NewController(informerSet.Core().V1().Pods(), informerSet.Batch().V1().Jobs(), clientSet)
	informerSet.Start(ctx.Done())
	go jc.Run(1, ctx.Done())
	return ctx, cancel
}

func hasJobTrackingFinalizer(obj metav1.Object) bool {
	for _, fin := range obj.GetFinalizers() {
		if fin == batchv1.JobTrackingFinalizer {
			return true
		}
	}
	return false
}

func hasJobTrackingAnnotation(job *batchv1.Job) bool {
	if job.Annotations == nil {
		return false
	}
	_, ok := job.Annotations[batchv1.JobTrackingFinalizer]
	return ok
}
