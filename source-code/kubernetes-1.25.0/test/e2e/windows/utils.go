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

package windows

import (
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/controller/deployment/util"
	"k8s.io/kubernetes/test/e2e/framework"

	semver "github.com/blang/semver/v4"
)

// waits for a deployment to be created and the desired replicas
// are updated and available, and no old pods are running.
func waitForDeployment(getDeploymentFunc func() (*appsv1.Deployment, error), interval, timeout time.Duration) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		deployment, err := getDeploymentFunc()
		if err != nil {
			if apierrors.IsNotFound(err) {
				framework.Logf("deployment not found, continue waiting: %s", err)
				return false, nil
			}

			framework.Logf("error while deploying, error %s", err)
			return false, err
		}
		framework.Logf("deployment status %s", &deployment.Status)
		return util.DeploymentComplete(deployment, &deployment.Status), nil
	})
}

// gets the container runtime and version for a node
func getNodeContainerRuntimeAndVersion(n v1.Node) (string, semver.Version, error) {
	containerRuntimeVersionString := n.Status.NodeInfo.DeepCopy().ContainerRuntimeVersion
	parts := strings.Split(containerRuntimeVersionString, "://")

	if len(parts) != 2 {
		return "", semver.Version{}, fmt.Errorf("could not get container runtime and version from '%s'", containerRuntimeVersionString)
	}

	v, err := semver.ParseTolerant(parts[1])
	if err != nil {
		return "", semver.Version{}, err
	}

	return parts[0], v, nil
}
