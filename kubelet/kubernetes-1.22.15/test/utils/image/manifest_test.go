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

package image

import (
	"fmt"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

// ToDo Add Benchmark
func TestReplaceRegistryInImageURL(t *testing.T) {
	registryTests := []struct {
		in        string
		out       string
		expectErr error
	}{
		{
			in:  "docker.io/library/test:123",
			out: "test.io/library/test:123",
		}, {
			in:  "docker.io/library/test",
			out: "test.io/library/test",
		}, {
			in:  "test",
			out: "test.io/library/test",
		}, {
			in:  "gcr.io/kubernetes-e2e-test-images/test:123",
			out: "test.io/kubernetes-e2e-test-images/test:123",
		}, {
			in:  "k8s.gcr.io/test:123",
			out: "test.io/test:123",
		}, {
			in:  "gcr.io/k8s-authenticated-test/test:123",
			out: "test.io/k8s-authenticated-test/test:123",
		}, {
			in:  "gcr.io/google-samples/test:latest",
			out: "test.io/google-samples/test:latest",
		}, {
			in:  "gcr.io/gke-release/test:latest",
			out: "test.io/gke-release/test:latest",
		}, {
			in:  "k8s.gcr.io/sig-storage/test:latest",
			out: "test.io/sig-storage/test:latest",
		}, {
			in:  "invalid.com/invalid/test:latest",
			out: "test.io/invalid/test:latest",
		}, {
			in:  "mcr.microsoft.com/test:latest",
			out: "test.io/microsoft/test:latest",
		}, {
			in:  "k8s.gcr.io/e2e-test-images/test:latest",
			out: "test.io/promoter/test:latest",
		}, {
			in:  "k8s.gcr.io/build-image/test:latest",
			out: "test.io/build/test:latest",
		}, {
			in:  "gcr.io/authenticated-image-pulling/test:latest",
			out: "test.io/gcAuth/test:latest",
		}, {
			in:        "unknwon.io/google-samples/test:latest",
			expectErr: fmt.Errorf("Registry: unknwon.io/google-samples is missing in test/utils/image/manifest.go, please add the registry, otherwise the test will fail on air-gapped clusters"),
		},
	}

	// Set custom registries
	reg := RegistryList{
		DockerLibraryRegistry:   "test.io/library",
		E2eRegistry:             "test.io/kubernetes-e2e-test-images",
		GcRegistry:              "test.io",
		GcrReleaseRegistry:      "test.io/gke-release",
		PrivateRegistry:         "test.io/k8s-authenticated-test",
		SampleRegistry:          "test.io/google-samples",
		SigStorageRegistry:      "test.io/sig-storage",
		InvalidRegistry:         "test.io/invalid",
		MicrosoftRegistry:       "test.io/microsoft",
		PromoterE2eRegistry:     "test.io/promoter",
		BuildImageRegistry:      "test.io/build",
		GcAuthenticatedRegistry: "test.io/gcAuth",
	}

	for _, tt := range registryTests {
		t.Run(tt.in, func(t *testing.T) {
			s, err := replaceRegistryInImageURLWithList(tt.in, reg)

			if err != nil && err.Error() != tt.expectErr.Error() {
				t.Errorf("got %q, want %q", err, tt.expectErr)
			}
			if s != tt.out {
				t.Errorf("got %q, want %q", s, tt.out)
			}
		})
	}
}

func TestGetOriginalImageConfigs(t *testing.T) {
	if len(GetOriginalImageConfigs()) == 0 {
		t.Fatalf("original map should not be empty")
	}
}

func TestGetMappedImageConfigs(t *testing.T) {
	originals := map[int]Config{
		0: {registry: "docker.io", name: "source/repo", version: "1.0"},
	}
	mapping := GetMappedImageConfigs(originals, "quay.io/repo/for-test")

	actual := make(map[string]string)
	for i, mapping := range mapping {
		source := originals[i]
		actual[source.GetE2EImage()] = mapping.GetE2EImage()
	}
	expected := map[string]string{
		"docker.io/source/repo:1.0": "quay.io/repo/for-test:e2e-0-docker-io-source-repo-1-0-72R4aXm7YnxQ4_ek",
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal(diff.ObjectReflectDiff(expected, actual))
	}
}
