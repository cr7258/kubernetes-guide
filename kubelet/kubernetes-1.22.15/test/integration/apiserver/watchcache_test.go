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

package apiserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/features"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/controlplane/reconcilers"
	"k8s.io/kubernetes/test/integration/framework"
)

// setup create kube-apiserver backed up by two separate etcds,
// with one of them containing events and the other all other objects.
func multiEtcdSetup(t testing.TB) (clientset.Interface, framework.CloseFunc) {
	etcdArgs := []string{"--experimental-watch-progress-notify-interval", "1s"}
	etcd0URL, stopEtcd0, err := framework.RunCustomEtcd("etcd_watchcache0", etcdArgs)
	if err != nil {
		t.Fatalf("Couldn't start etcd: %v", err)
	}

	etcd1URL, stopEtcd1, err := framework.RunCustomEtcd("etcd_watchcache1", etcdArgs)
	if err != nil {
		t.Fatalf("Couldn't start etcd: %v", err)
	}

	etcdOptions := framework.DefaultEtcdOptions()
	// Overwrite etcd setup to our custom etcd instances.
	etcdOptions.StorageConfig.Transport.ServerList = []string{etcd0URL}
	etcdOptions.EtcdServersOverrides = []string{fmt.Sprintf("/events#%s", etcd1URL)}
	etcdOptions.EnableWatchCache = true

	opts := framework.ControlPlaneConfigOptions{EtcdOptions: etcdOptions}
	controlPlaneConfig := framework.NewIntegrationTestControlPlaneConfigWithOptions(&opts)
	// Switch off endpoints reconciler to avoid unnecessary operations.
	controlPlaneConfig.ExtraConfig.EndpointReconcilerType = reconcilers.NoneEndpointReconcilerType
	_, s, stopAPIServer := framework.RunAnAPIServer(controlPlaneConfig)

	closeFn := func() {
		stopAPIServer()
		stopEtcd1()
		stopEtcd0()
	}

	clientSet, err := clientset.NewForConfig(&restclient.Config{Host: s.URL, QPS: -1})
	if err != nil {
		t.Fatalf("Error in create clientset: %v", err)
	}

	// Wait for apiserver to be stabilized.
	// Everything but default service creation is checked in RunAnAPIServer above by
	// waiting for post start hooks, so we just wait for default service to exist.
	// TODO(wojtek-t): Figure out less fragile way.
	ctx := context.Background()
	if err := wait.Poll(100*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
		_, err := clientSet.CoreV1().Services("default").Get(ctx, "kubernetes", metav1.GetOptions{})
		return err == nil, nil
	}); err != nil {
		t.Fatalf("Failed to wait for kubernetes service: %v:", err)
	}
	return clientSet, closeFn
}

func TestWatchCacheUpdatedByEtcd(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EfficientWatchResumption, true)()

	c, closeFn := multiEtcdSetup(t)
	defer closeFn()

	ctx := context.Background()

	makeConfigMap := func(name string) *v1.ConfigMap {
		return &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}
	makeSecret := func(name string) *v1.Secret {
		return &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}
	makeEvent := func(name string) *v1.Event {
		return &v1.Event{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	cm, err := c.CoreV1().ConfigMaps("default").Create(ctx, makeConfigMap("name"), metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Couldn't create configmap: %v", err)
	}
	ev, err := c.CoreV1().Events("default").Create(ctx, makeEvent("name"), metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Couldn't create event: %v", err)
	}

	listOptions := metav1.ListOptions{
		ResourceVersion:      "0",
		ResourceVersionMatch: metav1.ResourceVersionMatchNotOlderThan,
	}

	// Wait until listing from cache returns resource version of corresponding
	// resources (being the last updates).
	t.Logf("Waiting for configmaps watchcache synced to %s", cm.ResourceVersion)
	if err := wait.Poll(100*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
		res, err := c.CoreV1().ConfigMaps("default").List(ctx, listOptions)
		if err != nil {
			return false, nil
		}
		return res.ResourceVersion == cm.ResourceVersion, nil
	}); err != nil {
		t.Errorf("Failed to wait for configmaps watchcache synced: %v", err)
	}
	t.Logf("Waiting for events watchcache synced to %s", ev.ResourceVersion)
	if err := wait.Poll(100*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
		res, err := c.CoreV1().Events("default").List(ctx, listOptions)
		if err != nil {
			return false, nil
		}
		return res.ResourceVersion == ev.ResourceVersion, nil
	}); err != nil {
		t.Errorf("Failed to wait for events watchcache synced: %v", err)
	}

	// Create a secret, that is stored in the same etcd as configmap, but
	// different than events.
	se, err := c.CoreV1().Secrets("default").Create(ctx, makeSecret("name"), metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Couldn't create secret: %v", err)
	}

	t.Logf("Waiting for configmaps watchcache synced to %s", se.ResourceVersion)
	if err := wait.Poll(100*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
		res, err := c.CoreV1().ConfigMaps("default").List(ctx, listOptions)
		if err != nil {
			return false, nil
		}
		return res.ResourceVersion == se.ResourceVersion, nil
	}); err != nil {
		t.Errorf("Failed to wait for configmaps watchcache synced: %v", err)
	}
	t.Logf("Waiting for events watchcache NOT synced to %s", se.ResourceVersion)
	if err := wait.Poll(100*time.Millisecond, 5*time.Second, func() (bool, error) {
		res, err := c.CoreV1().Events("default").List(ctx, listOptions)
		if err != nil {
			return false, nil
		}
		return res.ResourceVersion == se.ResourceVersion, nil
	}); err == nil || err != wait.ErrWaitTimeout {
		t.Errorf("Events watchcache unexpected synced: %v", err)
	}
}
