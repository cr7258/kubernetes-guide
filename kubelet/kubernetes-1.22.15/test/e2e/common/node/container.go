/*
Copyright 2016 The Kubernetes Authors.

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

package node

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	// ContainerStatusRetryTimeout represents polling threshold before giving up to get the container status
	ContainerStatusRetryTimeout = time.Minute * 5
	// ContainerStatusPollInterval represents duration between polls to get the container status
	ContainerStatusPollInterval = time.Second * 1
)

// ConformanceContainer defines the types for running container conformance test cases
// One pod one container
type ConformanceContainer struct {
	Container        v1.Container
	RestartPolicy    v1.RestartPolicy
	Volumes          []v1.Volume
	ImagePullSecrets []string

	PodClient          *framework.PodClient
	podName            string
	PodSecurityContext *v1.PodSecurityContext
}

// Create creates the defined conformance container
func (cc *ConformanceContainer) Create() {
	cc.podName = cc.Container.Name + string(uuid.NewUUID())
	imagePullSecrets := []v1.LocalObjectReference{}
	for _, s := range cc.ImagePullSecrets {
		imagePullSecrets = append(imagePullSecrets, v1.LocalObjectReference{Name: s})
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: cc.podName,
		},
		Spec: v1.PodSpec{
			RestartPolicy: cc.RestartPolicy,
			Containers: []v1.Container{
				cc.Container,
			},
			SecurityContext:  cc.PodSecurityContext,
			Volumes:          cc.Volumes,
			ImagePullSecrets: imagePullSecrets,
		},
	}
	cc.PodClient.Create(pod)
}

// Delete deletes the defined conformance container
func (cc *ConformanceContainer) Delete() error {
	return cc.PodClient.Delete(context.TODO(), cc.podName, *metav1.NewDeleteOptions(0))
}

// IsReady returns whether this container is ready and error if any
func (cc *ConformanceContainer) IsReady() (bool, error) {
	pod, err := cc.PodClient.Get(context.TODO(), cc.podName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return podutil.IsPodReady(pod), nil
}

// GetPhase returns the phase of the pod lifecycle and error if any
func (cc *ConformanceContainer) GetPhase() (v1.PodPhase, error) {
	pod, err := cc.PodClient.Get(context.TODO(), cc.podName, metav1.GetOptions{})
	if err != nil {
		// it doesn't matter what phase to return as error would not be nil
		return v1.PodSucceeded, err
	}
	return pod.Status.Phase, nil
}

// GetStatus returns the details of the current status of this container and error if any
func (cc *ConformanceContainer) GetStatus() (v1.ContainerStatus, error) {
	pod, err := cc.PodClient.Get(context.TODO(), cc.podName, metav1.GetOptions{})
	if err != nil {
		return v1.ContainerStatus{}, err
	}
	statuses := pod.Status.ContainerStatuses
	if len(statuses) != 1 || statuses[0].Name != cc.Container.Name {
		return v1.ContainerStatus{}, fmt.Errorf("unexpected container statuses %v", statuses)
	}
	return statuses[0], nil
}

// Present returns whether this pod is present and error if any
func (cc *ConformanceContainer) Present() (bool, error) {
	_, err := cc.PodClient.Get(context.TODO(), cc.podName, metav1.GetOptions{})
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

// ContainerState represents different states of its lifecycle
type ContainerState string

const (
	// ContainerStateWaiting represents 'Waiting' container state
	ContainerStateWaiting ContainerState = "Waiting"
	// ContainerStateRunning represents 'Running' container state
	ContainerStateRunning ContainerState = "Running"
	// ContainerStateTerminated represents 'Terminated' container state
	ContainerStateTerminated ContainerState = "Terminated"
	// ContainerStateUnknown represents 'Unknown' container state
	ContainerStateUnknown ContainerState = "Unknown"
)

// GetContainerState returns current state the container represents among its lifecyle
func GetContainerState(state v1.ContainerState) ContainerState {
	if state.Waiting != nil {
		return ContainerStateWaiting
	}
	if state.Running != nil {
		return ContainerStateRunning
	}
	if state.Terminated != nil {
		return ContainerStateTerminated
	}
	return ContainerStateUnknown
}
