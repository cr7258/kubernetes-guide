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

package metrics

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/prometheus/common/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/component-base/metrics/testutil"
	"k8s.io/kubernetes/test/integration/framework"
)

func scrapeMetrics(s *httptest.Server) (testutil.Metrics, error) {
	req, err := http.NewRequest("GET", s.URL+"/metrics", nil)
	if err != nil {
		return nil, fmt.Errorf("Unable to create http request: %v", err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Unable to contact metrics endpoint of API server: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Non-200 response trying to scrape metrics from API Server: %v", resp)
	}
	metrics := testutil.NewMetrics()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Unable to read response: %v", resp)
	}
	err = testutil.ParseMetrics(string(data), &metrics)
	return metrics, err
}

func checkForExpectedMetrics(t *testing.T, metrics testutil.Metrics, expectedMetrics []string) {
	for _, expected := range expectedMetrics {
		if _, found := metrics[expected]; !found {
			t.Errorf("API server metrics did not include expected metric %q", expected)
		}
	}
}

func TestAPIServerProcessMetrics(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skipf("not supported on GOOS=%s", runtime.GOOS)
	}

	_, s, closeFn := framework.RunAnAPIServer(nil)
	defer closeFn()

	metrics, err := scrapeMetrics(s)
	if err != nil {
		t.Fatal(err)
	}
	checkForExpectedMetrics(t, metrics, []string{
		"process_start_time_seconds",
		"process_cpu_seconds_total",
		"process_open_fds",
		"process_resident_memory_bytes",
	})
}

func TestAPIServerMetrics(t *testing.T) {
	_, s, closeFn := framework.RunAnAPIServer(nil)
	defer closeFn()

	// Make a request to the apiserver to ensure there's at least one data point
	// for the metrics we're expecting -- otherwise, they won't be exported.
	client := clientset.NewForConfigOrDie(&restclient.Config{Host: s.URL, ContentConfig: restclient.ContentConfig{GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"}}})
	if _, err := client.CoreV1().Pods(metav1.NamespaceDefault).List(context.TODO(), metav1.ListOptions{}); err != nil {
		t.Fatalf("unexpected error getting pods: %v", err)
	}

	// Make a request to a deprecated API to ensure there's at least one data point
	if _, err := client.PolicyV1beta1().PodSecurityPolicies().List(context.TODO(), metav1.ListOptions{}); err != nil {
		t.Fatalf("unexpected error getting rbac roles: %v", err)
	}

	metrics, err := scrapeMetrics(s)
	if err != nil {
		t.Fatal(err)
	}
	checkForExpectedMetrics(t, metrics, []string{
		"apiserver_requested_deprecated_apis",
		"apiserver_request_total",
		"apiserver_request_duration_seconds_sum",
		"etcd_request_duration_seconds_sum",
	})
}

func TestAPIServerMetricsLabels(t *testing.T) {
	_, s, closeFn := framework.RunAnAPIServer(nil)
	defer closeFn()

	client, err := clientset.NewForConfig(&restclient.Config{Host: s.URL, QPS: -1})
	if err != nil {
		t.Fatalf("Error in create clientset: %v", err)
	}

	expectedMetrics := []model.Metric{}

	metricLabels := func(group, version, resource, subresource, scope, verb string) model.Metric {
		return map[model.LabelName]model.LabelValue{
			model.LabelName("group"):       model.LabelValue(group),
			model.LabelName("version"):     model.LabelValue(version),
			model.LabelName("resource"):    model.LabelValue(resource),
			model.LabelName("subresource"): model.LabelValue(subresource),
			model.LabelName("scope"):       model.LabelValue(scope),
			model.LabelName("verb"):        model.LabelValue(verb),
		}
	}

	callOrDie := func(_ interface{}, err error) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	appendExpectedMetric := func(metric model.Metric) {
		expectedMetrics = append(expectedMetrics, metric)
	}

	// Call appropriate endpoints to ensure particular metrics will be exposed

	// Namespace-scoped resource
	c := client.CoreV1().Pods(metav1.NamespaceDefault)
	makePod := func(labelValue string) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo",
				Labels: map[string]string{"foo": labelValue},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "container",
						Image: "image",
					},
				},
			},
		}
	}

	callOrDie(c.Create(context.TODO(), makePod("foo"), metav1.CreateOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "pods", "", "resource", "POST"))
	callOrDie(c.Update(context.TODO(), makePod("bar"), metav1.UpdateOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "pods", "", "resource", "PUT"))
	callOrDie(c.UpdateStatus(context.TODO(), makePod("bar"), metav1.UpdateOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "pods", "status", "resource", "PUT"))
	callOrDie(c.Get(context.TODO(), "foo", metav1.GetOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "pods", "", "resource", "GET"))
	callOrDie(c.List(context.TODO(), metav1.ListOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "pods", "", "namespace", "LIST"))
	callOrDie(nil, c.Delete(context.TODO(), "foo", metav1.DeleteOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "pods", "", "resource", "DELETE"))
	// cluster-scoped LIST of namespace-scoped resources
	callOrDie(client.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "pods", "", "cluster", "LIST"))

	// Cluster-scoped resource
	cn := client.CoreV1().Namespaces()
	makeNamespace := func(labelValue string) *v1.Namespace {
		return &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo",
				Labels: map[string]string{"foo": labelValue},
			},
		}
	}

	callOrDie(cn.Create(context.TODO(), makeNamespace("foo"), metav1.CreateOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "namespaces", "", "resource", "POST"))
	callOrDie(cn.Update(context.TODO(), makeNamespace("bar"), metav1.UpdateOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "namespaces", "", "resource", "PUT"))
	callOrDie(cn.UpdateStatus(context.TODO(), makeNamespace("bar"), metav1.UpdateOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "namespaces", "status", "resource", "PUT"))
	callOrDie(cn.Get(context.TODO(), "foo", metav1.GetOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "namespaces", "", "resource", "GET"))
	callOrDie(cn.List(context.TODO(), metav1.ListOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "namespaces", "", "cluster", "LIST"))
	callOrDie(nil, cn.Delete(context.TODO(), "foo", metav1.DeleteOptions{}))
	appendExpectedMetric(metricLabels("", "v1", "namespaces", "", "resource", "DELETE"))

	// Verify if all metrics were properly exported.
	metrics, err := scrapeMetrics(s)
	if err != nil {
		t.Fatal(err)
	}

	samples, ok := metrics["apiserver_request_total"]
	if !ok {
		t.Fatalf("apiserver_request_total metric not exposed")
	}

	hasLabels := func(current, expected model.Metric) bool {
		for key, value := range expected {
			if current[key] != value {
				return false
			}
		}
		return true
	}

	for _, expectedMetric := range expectedMetrics {
		found := false
		for _, sample := range samples {
			if hasLabels(sample.Metric, expectedMetric) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("No sample found for %#v", expectedMetric)
		}
	}
}

func TestAPIServerMetricsPods(t *testing.T) {
	callOrDie := func(_ interface{}, err error) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	makePod := func(labelValue string) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo",
				Labels: map[string]string{"foo": labelValue},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "container",
						Image: "image",
					},
				},
			},
		}
	}

	_, server, closeFn := framework.RunAnAPIServer(framework.NewControlPlaneConfig())
	defer closeFn()

	client, err := clientset.NewForConfig(&restclient.Config{Host: server.URL, QPS: -1})
	if err != nil {
		t.Fatalf("Error in create clientset: %v", err)
	}

	c := client.CoreV1().Pods(metav1.NamespaceDefault)

	for _, tc := range []struct {
		name     string
		executor func()

		want string
	}{
		{
			name: "create pod",
			executor: func() {
				callOrDie(c.Create(context.TODO(), makePod("foo"), metav1.CreateOptions{}))
			},
			want: `apiserver_request_total{code="201", component="apiserver", dry_run="", group="", resource="pods", scope="resource", subresource="", verb="POST", version="v1"}`,
		},
		{
			name: "update pod",
			executor: func() {
				callOrDie(c.Update(context.TODO(), makePod("bar"), metav1.UpdateOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="pods", scope="resource", subresource="", verb="PUT", version="v1"}`,
		},
		{
			name: "update pod status",
			executor: func() {
				callOrDie(c.UpdateStatus(context.TODO(), makePod("bar"), metav1.UpdateOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="pods", scope="resource", subresource="status", verb="PUT", version="v1"}`,
		},
		{
			name: "get pod",
			executor: func() {
				callOrDie(c.Get(context.TODO(), "foo", metav1.GetOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="pods", scope="resource", subresource="", verb="GET", version="v1"}`,
		},
		{
			name: "list pod",
			executor: func() {
				callOrDie(c.List(context.TODO(), metav1.ListOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="pods", scope="namespace", subresource="", verb="LIST", version="v1"}`,
		},
		{
			name: "delete pod",
			executor: func() {
				callOrDie(nil, c.Delete(context.TODO(), "foo", metav1.DeleteOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="pods", scope="resource", subresource="", verb="DELETE", version="v1"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {

			baseSamples, err := getSamples(server)
			if err != nil {
				t.Fatal(err)
			}

			tc.executor()

			updatedSamples, err := getSamples(server)
			if err != nil {
				t.Fatal(err)
			}

			newSamples := diffMetrics(updatedSamples, baseSamples)
			found := false

			for _, sample := range newSamples {
				if sample.Metric.String() == tc.want {
					found = true
					break
				}
			}

			if !found {
				t.Fatalf("could not find metric for API call >%s< among samples >%+v<", tc.name, newSamples)
			}
		})
	}
}

func TestAPIServerMetricsNamespaces(t *testing.T) {
	callOrDie := func(_ interface{}, err error) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	makeNamespace := func(labelValue string) *v1.Namespace {
		return &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo",
				Labels: map[string]string{"foo": labelValue},
			},
		}
	}

	_, server, closeFn := framework.RunAnAPIServer(framework.NewControlPlaneConfig())
	defer closeFn()

	client, err := clientset.NewForConfig(&restclient.Config{Host: server.URL, QPS: -1})
	if err != nil {
		t.Fatalf("Error in create clientset: %v", err)
	}

	c := client.CoreV1().Namespaces()

	for _, tc := range []struct {
		name     string
		executor func()

		want string
	}{
		{
			name: "create namespace",
			executor: func() {
				callOrDie(c.Create(context.TODO(), makeNamespace("foo"), metav1.CreateOptions{}))
			},
			want: `apiserver_request_total{code="201", component="apiserver", dry_run="", group="", resource="namespaces", scope="resource", subresource="", verb="POST", version="v1"}`,
		},
		{
			name: "update namespace",
			executor: func() {
				callOrDie(c.Update(context.TODO(), makeNamespace("bar"), metav1.UpdateOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="namespaces", scope="resource", subresource="", verb="PUT", version="v1"}`,
		},
		{
			name: "update namespace status",
			executor: func() {
				callOrDie(c.UpdateStatus(context.TODO(), makeNamespace("bar"), metav1.UpdateOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="namespaces", scope="resource", subresource="status", verb="PUT", version="v1"}`,
		},
		{
			name: "get namespace",
			executor: func() {
				callOrDie(c.Get(context.TODO(), "foo", metav1.GetOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="namespaces", scope="resource", subresource="", verb="GET", version="v1"}`,
		},
		{
			name: "list namespace",
			executor: func() {
				callOrDie(c.List(context.TODO(), metav1.ListOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="namespaces", scope="cluster", subresource="", verb="LIST", version="v1"}`,
		},
		{
			name: "delete namespace",
			executor: func() {
				callOrDie(nil, c.Delete(context.TODO(), "foo", metav1.DeleteOptions{}))
			},
			want: `apiserver_request_total{code="200", component="apiserver", dry_run="", group="", resource="namespaces", scope="resource", subresource="", verb="DELETE", version="v1"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {

			baseSamples, err := getSamples(server)
			if err != nil {
				t.Fatal(err)
			}

			tc.executor()

			updatedSamples, err := getSamples(server)
			if err != nil {
				t.Fatal(err)
			}

			newSamples := diffMetrics(updatedSamples, baseSamples)
			found := false

			for _, sample := range newSamples {
				if sample.Metric.String() == tc.want {
					found = true
					break
				}
			}

			if !found {
				t.Fatalf("could not find metric for API call >%s< among samples >%+v<", tc.name, newSamples)
			}
		})
	}
}

func getSamples(s *httptest.Server) (model.Samples, error) {
	metrics, err := scrapeMetrics(s)
	if err != nil {
		return nil, err
	}

	samples, ok := metrics["apiserver_request_total"]
	if !ok {
		return nil, errors.New("apiserver_request_total doesn't exist")
	}
	return samples, nil
}

func diffMetrics(newSamples model.Samples, oldSamples model.Samples) model.Samples {
	samplesDiff := model.Samples{}
	for _, sample := range newSamples {
		if !sampleExistsInSamples(sample, oldSamples) {
			samplesDiff = append(samplesDiff, sample)
		}
	}
	return samplesDiff
}

func sampleExistsInSamples(s *model.Sample, samples model.Samples) bool {
	for _, sample := range samples {
		if sample.Equal(s) {
			return true
		}
	}
	return false
}
