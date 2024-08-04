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

package scheduler

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/controller/testutil"
	"k8s.io/kubernetes/pkg/features"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var timeForControllerToProgress = 500 * time.Millisecond

func getPodsAssignedToNode(c *fake.Clientset) GetPodsByNodeNameFunc {
	return func(nodeName string) ([]*v1.Pod, error) {
		selector := fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName})
		pods, err := c.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{
			FieldSelector: selector.String(),
			LabelSelector: labels.Everything().String(),
		})
		if err != nil {
			return []*v1.Pod{}, fmt.Errorf("failed to get Pods assigned to node %v", nodeName)
		}
		rPods := make([]*v1.Pod, len(pods.Items))
		for i := range pods.Items {
			rPods[i] = &pods.Items[i]
		}
		return rPods, nil
	}
}

func createNoExecuteTaint(index int) v1.Taint {
	now := metav1.Now()
	return v1.Taint{
		Key:       "testTaint" + fmt.Sprintf("%v", index),
		Value:     "test" + fmt.Sprintf("%v", index),
		Effect:    v1.TaintEffectNoExecute,
		TimeAdded: &now,
	}
}

func addToleration(pod *v1.Pod, index int, duration int64) *v1.Pod {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	if duration < 0 {
		pod.Spec.Tolerations = []v1.Toleration{{Key: "testTaint" + fmt.Sprintf("%v", index), Value: "test" + fmt.Sprintf("%v", index), Effect: v1.TaintEffectNoExecute}}

	} else {
		pod.Spec.Tolerations = []v1.Toleration{{Key: "testTaint" + fmt.Sprintf("%v", index), Value: "test" + fmt.Sprintf("%v", index), Effect: v1.TaintEffectNoExecute, TolerationSeconds: &duration}}
	}
	return pod
}

func addTaintsToNode(node *v1.Node, key, value string, indices []int) *v1.Node {
	taints := []v1.Taint{}
	for _, index := range indices {
		taints = append(taints, createNoExecuteTaint(index))
	}
	node.Spec.Taints = taints
	return node
}

func setupNewNoExecuteTaintManager(ctx context.Context, fakeClientSet *fake.Clientset) (*NoExecuteTaintManager, cache.Indexer, cache.Indexer) {
	informerFactory := informers.NewSharedInformerFactory(fakeClientSet, 0)
	podIndexer := informerFactory.Core().V1().Pods().Informer().GetIndexer()
	nodeIndexer := informerFactory.Core().V1().Nodes().Informer().GetIndexer()
	mgr := NewNoExecuteTaintManager(ctx, fakeClientSet, informerFactory.Core().V1().Pods().Lister(), informerFactory.Core().V1().Nodes().Lister(), getPodsAssignedToNode(fakeClientSet))
	return mgr, podIndexer, nodeIndexer
}

type timestampedPod struct {
	names     []string
	timestamp time.Duration
}

type durationSlice []timestampedPod

func (a durationSlice) Len() int           { return len(a) }
func (a durationSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a durationSlice) Less(i, j int) bool { return a[i].timestamp < a[j].timestamp }

func TestFilterNoExecuteTaints(t *testing.T) {
	taints := []v1.Taint{
		{
			Key:    "one",
			Value:  "one",
			Effect: v1.TaintEffectNoExecute,
		},
		{
			Key:    "two",
			Value:  "two",
			Effect: v1.TaintEffectNoSchedule,
		},
	}
	taints = getNoExecuteTaints(taints)
	if len(taints) != 1 || taints[0].Key != "one" {
		t.Errorf("Filtering doesn't work. Got %v", taints)
	}
}

func TestCreatePod(t *testing.T) {
	testCases := []struct {
		description                   string
		pod                           *v1.Pod
		taintedNodes                  map[string][]v1.Taint
		expectPatch                   bool
		expectDelete                  bool
		enablePodDisruptionConditions bool
	}{
		{
			description:  "not scheduled - ignore",
			pod:          testutil.NewPod("pod1", ""),
			taintedNodes: map[string][]v1.Taint{},
			expectDelete: false,
		},
		{
			description:  "scheduled on untainted Node",
			pod:          testutil.NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{},
			expectDelete: false,
		},
		{
			description: "schedule on tainted Node",
			pod:         testutil.NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
		{
			description: "schedule on tainted Node; PodDisruptionConditions enabled",
			pod:         testutil.NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectPatch:                   true,
			expectDelete:                  true,
			enablePodDisruptionConditions: true,
		},
		{
			description: "schedule on tainted Node with finite toleration",
			pod:         addToleration(testutil.NewPod("pod1", "node1"), 1, 100),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: false,
		},
		{
			description: "schedule on tainted Node with infinite toleration",
			pod:         addToleration(testutil.NewPod("pod1", "node1"), 1, -1),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: false,
		},
		{
			description: "schedule on tainted Node with infinite ivalid toleration",
			pod:         addToleration(testutil.NewPod("pod1", "node1"), 2, -1),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
	}

	for _, item := range testCases {
		t.Run(item.description, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodDisruptionConditions, item.enablePodDisruptionConditions)()
			ctx, cancel := context.WithCancel(context.Background())
			fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*item.pod}})
			controller, podIndexer, _ := setupNewNoExecuteTaintManager(ctx, fakeClientset)
			controller.recorder = testutil.NewFakeRecorder()
			go controller.Run(ctx)
			controller.taintedNodes = item.taintedNodes

			podIndexer.Add(item.pod)
			controller.PodUpdated(nil, item.pod)
			// wait a bit
			time.Sleep(timeForControllerToProgress)

			verifyPodActions(t, item.description, fakeClientset, item.expectPatch, item.expectDelete)

			cancel()
		})
	}
}

func TestDeletePod(t *testing.T) {
	stopCh := make(chan struct{})
	fakeClientset := fake.NewSimpleClientset()
	controller, _, _ := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
	controller.recorder = testutil.NewFakeRecorder()
	go controller.Run(context.TODO())
	controller.taintedNodes = map[string][]v1.Taint{
		"node1": {createNoExecuteTaint(1)},
	}
	controller.PodUpdated(testutil.NewPod("pod1", "node1"), nil)
	// wait a bit to see if nothing will panic
	time.Sleep(timeForControllerToProgress)
	close(stopCh)
}

func TestUpdatePod(t *testing.T) {
	testCases := []struct {
		description                   string
		prevPod                       *v1.Pod
		newPod                        *v1.Pod
		taintedNodes                  map[string][]v1.Taint
		expectPatch                   bool
		expectDelete                  bool
		additionalSleep               time.Duration
		enablePodDisruptionConditions bool
	}{
		{
			description: "scheduling onto tainted Node results in patch and delete when PodDisruptionConditions enabled",
			prevPod:     testutil.NewPod("pod1", ""),
			newPod:      testutil.NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectPatch:                   true,
			expectDelete:                  true,
			enablePodDisruptionConditions: true,
		},
		{
			description: "scheduling onto tainted Node",
			prevPod:     testutil.NewPod("pod1", ""),
			newPod:      testutil.NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
		{
			description: "scheduling onto tainted Node with toleration",
			prevPod:     addToleration(testutil.NewPod("pod1", ""), 1, -1),
			newPod:      addToleration(testutil.NewPod("pod1", "node1"), 1, -1),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: false,
		},
		{
			description: "removing toleration",
			prevPod:     addToleration(testutil.NewPod("pod1", "node1"), 1, 100),
			newPod:      testutil.NewPod("pod1", "node1"),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete: true,
		},
		{
			description: "lengthening toleration shouldn't work",
			prevPod:     addToleration(testutil.NewPod("pod1", "node1"), 1, 1),
			newPod:      addToleration(testutil.NewPod("pod1", "node1"), 1, 100),
			taintedNodes: map[string][]v1.Taint{
				"node1": {createNoExecuteTaint(1)},
			},
			expectDelete:    true,
			additionalSleep: 1500 * time.Millisecond,
		},
	}

	for _, item := range testCases {
		t.Run(item.description, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodDisruptionConditions, item.enablePodDisruptionConditions)()
			ctx, cancel := context.WithCancel(context.Background())
			fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*item.prevPod}})
			controller, podIndexer, _ := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
			controller.recorder = testutil.NewFakeRecorder()
			go controller.Run(ctx)
			controller.taintedNodes = item.taintedNodes

			podIndexer.Add(item.prevPod)
			controller.PodUpdated(nil, item.prevPod)

			fakeClientset.ClearActions()
			time.Sleep(timeForControllerToProgress)
			podIndexer.Update(item.newPod)
			controller.PodUpdated(item.prevPod, item.newPod)
			// wait a bit
			time.Sleep(timeForControllerToProgress)
			if item.additionalSleep > 0 {
				time.Sleep(item.additionalSleep)
			}

			verifyPodActions(t, item.description, fakeClientset, item.expectPatch, item.expectDelete)
			cancel()
		})
	}
}

func TestCreateNode(t *testing.T) {
	testCases := []struct {
		description  string
		pods         []v1.Pod
		node         *v1.Node
		expectDelete bool
	}{
		{
			description: "Creating Node matching already assigned Pod",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
			},
			node:         testutil.NewNode("node1"),
			expectDelete: false,
		},
		{
			description: "Creating tainted Node matching already assigned Pod",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
			},
			node:         addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: true,
		},
		{
			description: "Creating tainted Node matching already assigned tolerating Pod",
			pods: []v1.Pod{
				*addToleration(testutil.NewPod("pod1", "node1"), 1, -1),
			},
			node:         addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: false,
		},
	}

	for _, item := range testCases {
		ctx, cancel := context.WithCancel(context.Background())
		fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: item.pods})
		controller, _, nodeIndexer := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
		nodeIndexer.Add(item.node)
		controller.recorder = testutil.NewFakeRecorder()
		go controller.Run(ctx)
		controller.NodeUpdated(nil, item.node)
		// wait a bit
		time.Sleep(timeForControllerToProgress)

		verifyPodActions(t, item.description, fakeClientset, false, item.expectDelete)

		cancel()
	}
}

func TestDeleteNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fakeClientset := fake.NewSimpleClientset()
	controller, _, _ := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
	controller.recorder = testutil.NewFakeRecorder()
	controller.taintedNodes = map[string][]v1.Taint{
		"node1": {createNoExecuteTaint(1)},
	}
	go controller.Run(ctx)
	controller.NodeUpdated(testutil.NewNode("node1"), nil)
	// wait a bit to see if nothing will panic
	time.Sleep(timeForControllerToProgress)
	controller.taintedNodesLock.Lock()
	if _, ok := controller.taintedNodes["node1"]; ok {
		t.Error("Node should have been deleted from taintedNodes list")
	}
	controller.taintedNodesLock.Unlock()
	cancel()
}

func TestUpdateNode(t *testing.T) {
	testCases := []struct {
		description                   string
		pods                          []v1.Pod
		oldNode                       *v1.Node
		newNode                       *v1.Node
		expectPatch                   bool
		expectDelete                  bool
		additionalSleep               time.Duration
		enablePodDisruptionConditions bool
	}{
		{
			description: "Added taint, expect node patched and deleted when PodDisruptionConditions is enabled",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
			},
			oldNode:                       testutil.NewNode("node1"),
			newNode:                       addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectPatch:                   true,
			expectDelete:                  true,
			enablePodDisruptionConditions: true,
		},
		{
			description: "Added taint",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
			},
			oldNode:      testutil.NewNode("node1"),
			newNode:      addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: true,
		},
		{
			description: "Added tolerated taint",
			pods: []v1.Pod{
				*addToleration(testutil.NewPod("pod1", "node1"), 1, 100),
			},
			oldNode:      testutil.NewNode("node1"),
			newNode:      addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: false,
		},
		{
			description: "Only one added taint tolerated",
			pods: []v1.Pod{
				*addToleration(testutil.NewPod("pod1", "node1"), 1, 100),
			},
			oldNode:      testutil.NewNode("node1"),
			newNode:      addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1, 2}),
			expectDelete: true,
		},
		{
			description: "Taint removed",
			pods: []v1.Pod{
				*addToleration(testutil.NewPod("pod1", "node1"), 1, 1),
			},
			oldNode:         addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			newNode:         testutil.NewNode("node1"),
			expectDelete:    false,
			additionalSleep: 1500 * time.Millisecond,
		},
		{
			description: "Pod with multiple tolerations are evicted when first one runs out",
			pods: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pod1",
					},
					Spec: v1.PodSpec{
						NodeName: "node1",
						Tolerations: []v1.Toleration{
							{Key: "testTaint1", Value: "test1", Effect: v1.TaintEffectNoExecute, TolerationSeconds: &[]int64{1}[0]},
							{Key: "testTaint2", Value: "test2", Effect: v1.TaintEffectNoExecute, TolerationSeconds: &[]int64{100}[0]},
						},
					},
					Status: v1.PodStatus{
						Conditions: []v1.PodCondition{
							{
								Type:   v1.PodReady,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			oldNode:         testutil.NewNode("node1"),
			newNode:         addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1, 2}),
			expectDelete:    true,
			additionalSleep: 1500 * time.Millisecond,
		},
	}

	for _, item := range testCases {
		t.Run(item.description, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.PodDisruptionConditions, item.enablePodDisruptionConditions)()
			stopCh := make(chan struct{})
			fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: item.pods})
			controller, _, nodeIndexer := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
			nodeIndexer.Add(item.newNode)
			controller.recorder = testutil.NewFakeRecorder()
			go controller.Run(context.TODO())
			controller.NodeUpdated(item.oldNode, item.newNode)
			// wait a bit
			time.Sleep(timeForControllerToProgress)
			if item.additionalSleep > 0 {
				time.Sleep(item.additionalSleep)
			}

			verifyPodActions(t, item.description, fakeClientset, item.expectPatch, item.expectDelete)
			close(stopCh)
		})
	}
}

func TestUpdateNodeWithMultipleTaints(t *testing.T) {
	taint1 := createNoExecuteTaint(1)
	taint2 := createNoExecuteTaint(2)

	minute := int64(60)
	pod := testutil.NewPod("pod1", "node1")
	pod.Spec.Tolerations = []v1.Toleration{
		{Key: taint1.Key, Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoExecute},
		{Key: taint2.Key, Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoExecute, TolerationSeconds: &minute},
	}
	podNamespacedName := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}

	untaintedNode := testutil.NewNode("node1")

	doubleTaintedNode := testutil.NewNode("node1")
	doubleTaintedNode.Spec.Taints = []v1.Taint{taint1, taint2}

	singleTaintedNode := testutil.NewNode("node1")
	singleTaintedNode.Spec.Taints = []v1.Taint{taint1}

	ctx, cancel := context.WithCancel(context.TODO())
	fakeClientset := fake.NewSimpleClientset(pod)
	controller, _, nodeIndexer := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
	controller.recorder = testutil.NewFakeRecorder()
	go controller.Run(context.TODO())

	// no taint
	nodeIndexer.Add(untaintedNode)
	controller.handleNodeUpdate(ctx, nodeUpdateItem{"node1"})
	// verify pod is not queued for deletion
	if controller.taintEvictionQueue.GetWorkerUnsafe(podNamespacedName.String()) != nil {
		t.Fatalf("pod queued for deletion with no taints")
	}

	// no taint -> infinitely tolerated taint
	nodeIndexer.Update(singleTaintedNode)
	controller.handleNodeUpdate(ctx, nodeUpdateItem{"node1"})
	// verify pod is not queued for deletion
	if controller.taintEvictionQueue.GetWorkerUnsafe(podNamespacedName.String()) != nil {
		t.Fatalf("pod queued for deletion with permanently tolerated taint")
	}

	// infinitely tolerated taint -> temporarily tolerated taint
	nodeIndexer.Update(doubleTaintedNode)
	controller.handleNodeUpdate(ctx, nodeUpdateItem{"node1"})
	// verify pod is queued for deletion
	if controller.taintEvictionQueue.GetWorkerUnsafe(podNamespacedName.String()) == nil {
		t.Fatalf("pod not queued for deletion after addition of temporarily tolerated taint")
	}

	// temporarily tolerated taint -> infinitely tolerated taint
	nodeIndexer.Update(singleTaintedNode)
	controller.handleNodeUpdate(ctx, nodeUpdateItem{"node1"})
	// verify pod is not queued for deletion
	if controller.taintEvictionQueue.GetWorkerUnsafe(podNamespacedName.String()) != nil {
		t.Fatalf("pod queued for deletion after removal of temporarily tolerated taint")
	}

	// verify pod is not deleted
	for _, action := range fakeClientset.Actions() {
		if action.GetVerb() == "delete" && action.GetResource().Resource == "pods" {
			t.Error("Unexpected deletion")
		}
	}
	cancel()
}

func TestUpdateNodeWithMultiplePods(t *testing.T) {
	testCases := []struct {
		description         string
		pods                []v1.Pod
		oldNode             *v1.Node
		newNode             *v1.Node
		expectedDeleteTimes durationSlice
	}{
		{
			description: "Pods with different toleration times are evicted appropriately",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
				*addToleration(testutil.NewPod("pod2", "node1"), 1, 1),
				*addToleration(testutil.NewPod("pod3", "node1"), 1, -1),
			},
			oldNode: testutil.NewNode("node1"),
			newNode: addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectedDeleteTimes: durationSlice{
				{[]string{"pod1"}, 0},
				{[]string{"pod2"}, time.Second},
			},
		},
		{
			description: "Evict all pods not matching all taints instantly",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
				*addToleration(testutil.NewPod("pod2", "node1"), 1, 1),
				*addToleration(testutil.NewPod("pod3", "node1"), 1, -1),
			},
			oldNode: testutil.NewNode("node1"),
			newNode: addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1, 2}),
			expectedDeleteTimes: durationSlice{
				{[]string{"pod1", "pod2", "pod3"}, 0},
			},
		},
	}

	for _, item := range testCases {
		t.Logf("Starting testcase %q", item.description)

		stopCh := make(chan struct{})
		fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: item.pods})
		sort.Sort(item.expectedDeleteTimes)
		controller, _, nodeIndexer := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
		nodeIndexer.Add(item.newNode)
		controller.recorder = testutil.NewFakeRecorder()
		go controller.Run(context.TODO())
		controller.NodeUpdated(item.oldNode, item.newNode)

		startedAt := time.Now()
		for i := range item.expectedDeleteTimes {
			if i == 0 || item.expectedDeleteTimes[i-1].timestamp != item.expectedDeleteTimes[i].timestamp {
				// compute a grace duration to give controller time to process updates. Choose big
				// enough intervals in the test cases above to avoid flakes.
				var increment time.Duration
				if i == len(item.expectedDeleteTimes)-1 || item.expectedDeleteTimes[i+1].timestamp == item.expectedDeleteTimes[i].timestamp {
					increment = 500 * time.Millisecond
				} else {
					increment = ((item.expectedDeleteTimes[i+1].timestamp - item.expectedDeleteTimes[i].timestamp) / time.Duration(2))
				}

				sleepTime := item.expectedDeleteTimes[i].timestamp - time.Since(startedAt) + increment
				if sleepTime < 0 {
					sleepTime = 0
				}
				t.Logf("Sleeping for %v", sleepTime)
				time.Sleep(sleepTime)
			}

			for delay, podName := range item.expectedDeleteTimes[i].names {
				deleted := false
				for _, action := range fakeClientset.Actions() {
					deleteAction, ok := action.(clienttesting.DeleteActionImpl)
					if !ok {
						t.Logf("Found not-delete action with verb %v. Ignoring.", action.GetVerb())
						continue
					}
					if deleteAction.GetResource().Resource != "pods" {
						continue
					}
					if podName == deleteAction.GetName() {
						deleted = true
					}
				}
				if !deleted {
					t.Errorf("Failed to deleted pod %v after %v", podName, delay)
				}
			}
			for _, action := range fakeClientset.Actions() {
				deleteAction, ok := action.(clienttesting.DeleteActionImpl)
				if !ok {
					t.Logf("Found not-delete action with verb %v. Ignoring.", action.GetVerb())
					continue
				}
				if deleteAction.GetResource().Resource != "pods" {
					continue
				}
				deletedPodName := deleteAction.GetName()
				expected := false
				for _, podName := range item.expectedDeleteTimes[i].names {
					if podName == deletedPodName {
						expected = true
					}
				}
				if !expected {
					t.Errorf("Pod %v was deleted even though it shouldn't have", deletedPodName)
				}
			}
			fakeClientset.ClearActions()
		}

		close(stopCh)
	}
}

func TestGetMinTolerationTime(t *testing.T) {
	one := int64(1)
	two := int64(2)
	oneSec := 1 * time.Second

	tests := []struct {
		tolerations []v1.Toleration
		expected    time.Duration
	}{
		{
			tolerations: []v1.Toleration{},
			expected:    0,
		},
		{
			tolerations: []v1.Toleration{
				{
					TolerationSeconds: nil,
				},
			},
			expected: -1,
		},
		{
			tolerations: []v1.Toleration{
				{
					TolerationSeconds: &one,
				},
				{
					TolerationSeconds: &two,
				},
			},
			expected: oneSec,
		},

		{
			tolerations: []v1.Toleration{
				{
					TolerationSeconds: &one,
				},
				{
					TolerationSeconds: nil,
				},
			},
			expected: oneSec,
		},
		{
			tolerations: []v1.Toleration{
				{
					TolerationSeconds: nil,
				},
				{
					TolerationSeconds: &one,
				},
			},
			expected: oneSec,
		},
	}

	for _, test := range tests {
		got := getMinTolerationTime(test.tolerations)
		if got != test.expected {
			t.Errorf("Incorrect min toleration time: got %v, expected %v", got, test.expected)
		}
	}
}

// TestEventualConsistency verifies if getPodsAssignedToNode returns incomplete data
// (e.g. due to watch latency), it will reconcile the remaining pods eventually.
// This scenario is partially covered by TestUpdatePods, but given this is an important
// property of TaintManager, it's better to have explicit test for this.
func TestEventualConsistency(t *testing.T) {
	testCases := []struct {
		description  string
		pods         []v1.Pod
		prevPod      *v1.Pod
		newPod       *v1.Pod
		oldNode      *v1.Node
		newNode      *v1.Node
		expectDelete bool
	}{
		{
			description: "existing pod2 scheduled onto tainted Node",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
			},
			prevPod:      testutil.NewPod("pod2", ""),
			newPod:       testutil.NewPod("pod2", "node1"),
			oldNode:      testutil.NewNode("node1"),
			newNode:      addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: true,
		},
		{
			description: "existing pod2 with taint toleration scheduled onto tainted Node",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
			},
			prevPod:      addToleration(testutil.NewPod("pod2", ""), 1, 100),
			newPod:       addToleration(testutil.NewPod("pod2", "node1"), 1, 100),
			oldNode:      testutil.NewNode("node1"),
			newNode:      addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: true,
		},
		{
			description: "new pod2 created on tainted Node",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
			},
			prevPod:      nil,
			newPod:       testutil.NewPod("pod2", "node1"),
			oldNode:      testutil.NewNode("node1"),
			newNode:      addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: true,
		},
		{
			description: "new pod2 with tait toleration created on tainted Node",
			pods: []v1.Pod{
				*testutil.NewPod("pod1", "node1"),
			},
			prevPod:      nil,
			newPod:       addToleration(testutil.NewPod("pod2", "node1"), 1, 100),
			oldNode:      testutil.NewNode("node1"),
			newNode:      addTaintsToNode(testutil.NewNode("node1"), "testTaint1", "taint1", []int{1}),
			expectDelete: true,
		},
	}

	for _, item := range testCases {
		stopCh := make(chan struct{})
		fakeClientset := fake.NewSimpleClientset(&v1.PodList{Items: item.pods})
		controller, podIndexer, nodeIndexer := setupNewNoExecuteTaintManager(context.TODO(), fakeClientset)
		nodeIndexer.Add(item.newNode)
		controller.recorder = testutil.NewFakeRecorder()
		go controller.Run(context.TODO())

		if item.prevPod != nil {
			podIndexer.Add(item.prevPod)
			controller.PodUpdated(nil, item.prevPod)
		}

		// First we simulate NodeUpdate that should delete 'pod1'. It doesn't know about 'pod2' yet.
		controller.NodeUpdated(item.oldNode, item.newNode)
		// TODO(mborsz): Remove this sleep and other sleeps in this file.
		time.Sleep(timeForControllerToProgress)

		verifyPodActions(t, item.description, fakeClientset, false, item.expectDelete)
		fakeClientset.ClearActions()

		// And now the delayed update of 'pod2' comes to the TaintManager. We should delete it as well.
		podIndexer.Update(item.newPod)
		controller.PodUpdated(item.prevPod, item.newPod)
		// wait a bit
		time.Sleep(timeForControllerToProgress)

		close(stopCh)
	}
}

func verifyPodActions(t *testing.T, description string, fakeClientset *fake.Clientset, expectPatch, expectDelete bool) {
	t.Helper()
	podPatched := false
	podDeleted := false
	for _, action := range fakeClientset.Actions() {
		if action.GetVerb() == "patch" && action.GetResource().Resource == "pods" {
			podPatched = true
		}
		if action.GetVerb() == "delete" && action.GetResource().Resource == "pods" {
			podDeleted = true
		}
	}
	if podPatched != expectPatch {
		t.Errorf("[%v]Unexpected test result. Expected patch %v, got %v", description, expectPatch, podPatched)
	}
	if podDeleted != expectDelete {
		t.Errorf("[%v]Unexpected test result. Expected delete %v, got %v", description, expectDelete, podDeleted)
	}
}
