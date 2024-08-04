/*
Copyright 2018 The Kubernetes Authors.

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

package options

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiserveroptions "k8s.io/apiserver/pkg/server/options"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/component-base/logs"
	v1 "k8s.io/kube-scheduler/config/v1"
	"k8s.io/kube-scheduler/config/v1beta2"
	"k8s.io/kube-scheduler/config/v1beta3"
	kubeschedulerconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/latest"
	configtesting "k8s.io/kubernetes/pkg/scheduler/apis/config/testing"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/testing/defaults"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/names"
)

func TestSchedulerOptions(t *testing.T) {
	// temp dir
	tmpDir, err := os.MkdirTemp("", "scheduler-options")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// record the username requests were made with
	username := ""
	// https server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		username, _, _ = req.BasicAuth()
		if username == "" {
			username = "none, tls"
		}
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()
	// http server
	insecureserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		username, _, _ = req.BasicAuth()
		if username == "" {
			username = "none, http"
		}
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer insecureserver.Close()

	// config file and kubeconfig
	configFile := filepath.Join(tmpDir, "scheduler.yaml")
	configKubeconfig := filepath.Join(tmpDir, "config.kubeconfig")
	if err := os.WriteFile(configFile, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
leaderElection:
  leaderElect: true`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configKubeconfig, []byte(fmt.Sprintf(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: %s
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
users:
- name: default
  user:
    username: config
`, server.URL)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	oldConfigFile := filepath.Join(tmpDir, "scheduler_old.yaml")
	if err := os.WriteFile(oldConfigFile, []byte(fmt.Sprintf(`
apiVersion: componentconfig/v1alpha1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
leaderElection:
  leaderElect: true`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	v1beta3VersionConfig := filepath.Join(tmpDir, "scheduler_v1beta3_api_version.yaml")
	if err := os.WriteFile(v1beta3VersionConfig, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1beta3
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
leaderElection:
  leaderElect: true`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	v1beta2VersionConfig := filepath.Join(tmpDir, "scheduler_v1beta2_api_version.yaml")
	if err := os.WriteFile(v1beta2VersionConfig, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1beta2
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
leaderElection:
  leaderElect: true`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	unknownVersionConfig := filepath.Join(tmpDir, "scheduler_invalid_wrong_api_version.yaml")
	if err := os.WriteFile(unknownVersionConfig, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/unknown
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
leaderElection:
  leaderElect: true`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	noVersionConfig := filepath.Join(tmpDir, "scheduler_invalid_no_version.yaml")
	if err := os.WriteFile(noVersionConfig, []byte(fmt.Sprintf(`
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
leaderElection:
  leaderElect: true`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	unknownFieldConfig := filepath.Join(tmpDir, "scheduler_invalid_unknown_field.yaml")
	if err := os.WriteFile(unknownFieldConfig, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
leaderElection:
  leaderElect: true
foo: bar`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	duplicateFieldConfig := filepath.Join(tmpDir, "scheduler_invalid_duplicate_fields.yaml")
	if err := os.WriteFile(duplicateFieldConfig, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
leaderElection:
  leaderElect: true
  leaderElect: false`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	// flag-specified kubeconfig
	flagKubeconfig := filepath.Join(tmpDir, "flag.kubeconfig")
	if err := os.WriteFile(flagKubeconfig, []byte(fmt.Sprintf(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: %s
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
users:
- name: default
  user:
    username: flag
`, server.URL)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	// plugin config
	pluginConfigFile := filepath.Join(tmpDir, "plugin.yaml")
	if err := os.WriteFile(pluginConfigFile, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
profiles:
- plugins:
    reserve:
      enabled:
      - name: foo
      - name: bar
      disabled:
      - name: VolumeBinding
    preBind:
      enabled:
      - name: foo
      disabled:
      - name: VolumeBinding
  pluginConfig:
  - name: InterPodAffinity
    args:
      hardPodAffinityWeight: 2
  - name: foo
    args:
      bar: baz
`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	// v1beta3 plugin config
	v1beta3PluginConfigFile := filepath.Join(tmpDir, "v1beta3_plugin.yaml")
	if err := os.WriteFile(v1beta3PluginConfigFile, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1beta3
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
profiles:
- plugins:
    reserve:
      enabled:
      - name: foo
      - name: bar
      disabled:
      - name: VolumeBinding
    preBind:
      enabled:
      - name: foo
      disabled:
      - name: VolumeBinding
  pluginConfig:
  - name: InterPodAffinity
    args:
      hardPodAffinityWeight: 2
  - name: foo
    args:
      bar: baz
`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	// v1beta2 plugin config
	v1beta2PluginConfigFile := filepath.Join(tmpDir, "v1beta2_plugin.yaml")
	if err := os.WriteFile(v1beta2PluginConfigFile, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1beta2
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
profiles:
- plugins:
    reserve:
      enabled:
      - name: foo
      - name: bar
      disabled:
      - name: VolumeBinding
    preBind:
      enabled:
      - name: foo
      disabled:
      - name: VolumeBinding
  pluginConfig:
  - name: InterPodAffinity
    args:
      hardPodAffinityWeight: 2
  - name: foo
    args:
      bar: baz
`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	// multiple profiles config
	multiProfilesConfig := filepath.Join(tmpDir, "multi-profiles.yaml")
	if err := os.WriteFile(multiProfilesConfig, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
profiles:
- schedulerName: "foo-profile"
  plugins:
    reserve:
      enabled:
      - name: foo
      - name: VolumeBinding
      disabled:
      - name: VolumeBinding
- schedulerName: "bar-profile"
  plugins:
    preBind:
      disabled:
      - name: VolumeBinding
  pluginConfig:
  - name: foo
`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	// v1beta3 multiple profiles config
	v1beta3MultiProfilesConfig := filepath.Join(tmpDir, "v1beta3_multi-profiles.yaml")
	if err := os.WriteFile(v1beta3MultiProfilesConfig, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1beta3
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
profiles:
- schedulerName: "foo-profile"
  plugins:
    reserve:
      enabled:
      - name: foo
      - name: VolumeBinding
      disabled:
      - name: VolumeBinding
- schedulerName: "bar-profile"
  plugins:
    preBind:
      disabled:
      - name: VolumeBinding
  pluginConfig:
  - name: foo
`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	// multiple profiles config
	v1beta2MultiProfilesConfig := filepath.Join(tmpDir, "v1beta2_multi-profiles.yaml")
	if err := os.WriteFile(v1beta2MultiProfilesConfig, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1beta2
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"
profiles:
- schedulerName: "foo-profile"
  plugins:
    reserve:
      enabled:
      - name: foo
      - name: VolumeBinding
      disabled:
      - name: VolumeBinding
- schedulerName: "bar-profile"
  plugins:
    preBind:
      disabled:
      - name: VolumeBinding
  pluginConfig:
  - name: foo
`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	// Insulate this test from picking up in-cluster config when run inside a pod
	// We can't assume we have permissions to write to /var/run/secrets/... from a unit test to mock in-cluster config for testing
	originalHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	if len(originalHost) > 0 {
		os.Setenv("KUBERNETES_SERVICE_HOST", "")
		defer os.Setenv("KUBERNETES_SERVICE_HOST", originalHost)
	}

	defaultPodInitialBackoffSeconds := int64(1)
	defaultPodMaxBackoffSeconds := int64(10)
	defaultPercentageOfNodesToScore := int32(0)

	testcases := []struct {
		name             string
		options          *Options
		expectedUsername string
		expectedError    string
		expectedConfig   kubeschedulerconfig.KubeSchedulerConfiguration
		checkErrFn       func(err error) bool
	}{
		{
			name: "v1 config file",
			options: &Options{
				ConfigFile: configFile,
				ComponentConfig: func() *kubeschedulerconfig.KubeSchedulerConfiguration {
					cfg := configtesting.V1ToInternalWithDefaults(t, v1.KubeSchedulerConfiguration{})
					return cfg
				}(),
				SecureServing: (&apiserveroptions.SecureServingOptions{
					ServerCert: apiserveroptions.GeneratableKeyCert{
						CertDirectory: "/a/b/c",
						PairName:      "kube-scheduler",
					},
					HTTP2MaxStreamsPerConnection: 47,
				}).WithLoopback(),
				Authentication: &apiserveroptions.DelegatingAuthenticationOptions{
					CacheTTL:   10 * time.Second,
					ClientCert: apiserveroptions.ClientCertAuthenticationOptions{},
					RequestHeader: apiserveroptions.RequestHeaderAuthenticationOptions{
						UsernameHeaders:     []string{"x-remote-user"},
						GroupHeaders:        []string{"x-remote-group"},
						ExtraHeaderPrefixes: []string{"x-remote-extra-"},
					},
					RemoteKubeConfigFileOptional: true,
				},
				Authorization: &apiserveroptions.DelegatingAuthorizationOptions{
					AllowCacheTTL:                10 * time.Second,
					DenyCacheTTL:                 10 * time.Second,
					RemoteKubeConfigFileOptional: true,
					AlwaysAllowPaths:             []string{"/healthz", "/readyz", "/livez"}, // note: this does not match /healthz/ or /healthz/*
					AlwaysAllowGroups:            []string{"system:masters"},
				},
				Logs: logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "default-scheduler",
						Plugins:       defaults.PluginsV1,
						PluginConfig:  defaults.PluginConfigsV1,
					},
				},
			},
		},
		{
			name: "v1beta3 config file",
			options: &Options{
				ConfigFile: v1beta3VersionConfig,
				ComponentConfig: func() *kubeschedulerconfig.KubeSchedulerConfiguration {
					cfg := configtesting.V1beta3ToInternalWithDefaults(t, v1beta3.KubeSchedulerConfiguration{})
					return cfg
				}(),
				SecureServing: (&apiserveroptions.SecureServingOptions{
					ServerCert: apiserveroptions.GeneratableKeyCert{
						CertDirectory: "/a/b/c",
						PairName:      "kube-scheduler",
					},
					HTTP2MaxStreamsPerConnection: 47,
				}).WithLoopback(),
				Authentication: &apiserveroptions.DelegatingAuthenticationOptions{
					CacheTTL:   10 * time.Second,
					ClientCert: apiserveroptions.ClientCertAuthenticationOptions{},
					RequestHeader: apiserveroptions.RequestHeaderAuthenticationOptions{
						UsernameHeaders:     []string{"x-remote-user"},
						GroupHeaders:        []string{"x-remote-group"},
						ExtraHeaderPrefixes: []string{"x-remote-extra-"},
					},
					RemoteKubeConfigFileOptional: true,
				},
				Authorization: &apiserveroptions.DelegatingAuthorizationOptions{
					AllowCacheTTL:                10 * time.Second,
					DenyCacheTTL:                 10 * time.Second,
					RemoteKubeConfigFileOptional: true,
					AlwaysAllowPaths:             []string{"/healthz", "/readyz", "/livez"}, // note: this does not match /healthz/ or /healthz/*
					AlwaysAllowGroups:            []string{"system:masters"},
				},
				Logs: logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta3.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "default-scheduler",
						Plugins:       defaults.PluginsV1beta3,
						PluginConfig:  defaults.PluginConfigsV1beta3,
					},
				},
			},
		},
		{
			name: "v1beta2 config file",
			options: &Options{
				ConfigFile: v1beta2VersionConfig,
				ComponentConfig: func() *kubeschedulerconfig.KubeSchedulerConfiguration {
					cfg := configtesting.V1beta2ToInternalWithDefaults(t, v1beta2.KubeSchedulerConfiguration{})
					return cfg
				}(),
				SecureServing: (&apiserveroptions.SecureServingOptions{
					ServerCert: apiserveroptions.GeneratableKeyCert{
						CertDirectory: "/a/b/c",
						PairName:      "kube-scheduler",
					},
					HTTP2MaxStreamsPerConnection: 47,
				}).WithLoopback(),
				Authentication: &apiserveroptions.DelegatingAuthenticationOptions{
					CacheTTL:   10 * time.Second,
					ClientCert: apiserveroptions.ClientCertAuthenticationOptions{},
					RequestHeader: apiserveroptions.RequestHeaderAuthenticationOptions{
						UsernameHeaders:     []string{"x-remote-user"},
						GroupHeaders:        []string{"x-remote-group"},
						ExtraHeaderPrefixes: []string{"x-remote-extra-"},
					},
					RemoteKubeConfigFileOptional: true,
				},
				Authorization: &apiserveroptions.DelegatingAuthorizationOptions{
					AllowCacheTTL:                10 * time.Second,
					DenyCacheTTL:                 10 * time.Second,
					RemoteKubeConfigFileOptional: true,
					AlwaysAllowPaths:             []string{"/healthz", "/readyz", "/livez"}, // note: this does not match /healthz/ or /healthz/*
					AlwaysAllowGroups:            []string{"system:masters"},
				},
				Logs: logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta2.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "default-scheduler",
						Plugins:       defaults.PluginsV1beta2,
						PluginConfig:  defaults.PluginConfigsV1beta2,
					},
				},
			},
		},
		{
			name: "config file in componentconfig/v1alpha1",
			options: &Options{
				ConfigFile: oldConfigFile,
				ComponentConfig: func() *kubeschedulerconfig.KubeSchedulerConfiguration {
					cfg, err := latest.Default()
					if err != nil {
						t.Fatal(err)
					}
					return cfg
				}(),
				Logs: logs.NewOptions(),
			},
			expectedError: "no kind \"KubeSchedulerConfiguration\" is registered for version \"componentconfig/v1alpha1\"",
		},
		{
			name: "unknown version kubescheduler.config.k8s.io/unknown",
			options: &Options{
				ConfigFile: unknownVersionConfig,
				Logs:       logs.NewOptions(),
			},
			expectedError: "no kind \"KubeSchedulerConfiguration\" is registered for version \"kubescheduler.config.k8s.io/unknown\"",
		},
		{
			name: "config file with no version",
			options: &Options{
				ConfigFile: noVersionConfig,
				Logs:       logs.NewOptions(),
			},
			expectedError: "Object 'apiVersion' is missing",
		},
		{
			name: "kubeconfig flag",
			options: &Options{
				ComponentConfig: func() *kubeschedulerconfig.KubeSchedulerConfiguration {
					cfg, _ := latest.Default()
					cfg.ClientConnection.Kubeconfig = flagKubeconfig
					return cfg
				}(),
				SecureServing: (&apiserveroptions.SecureServingOptions{
					ServerCert: apiserveroptions.GeneratableKeyCert{
						CertDirectory: "/a/b/c",
						PairName:      "kube-scheduler",
					},
					HTTP2MaxStreamsPerConnection: 47,
				}).WithLoopback(),
				Authentication: &apiserveroptions.DelegatingAuthenticationOptions{
					CacheTTL:   10 * time.Second,
					ClientCert: apiserveroptions.ClientCertAuthenticationOptions{},
					RequestHeader: apiserveroptions.RequestHeaderAuthenticationOptions{
						UsernameHeaders:     []string{"x-remote-user"},
						GroupHeaders:        []string{"x-remote-group"},
						ExtraHeaderPrefixes: []string{"x-remote-extra-"},
					},
					RemoteKubeConfigFileOptional: true,
				},
				Authorization: &apiserveroptions.DelegatingAuthorizationOptions{
					AllowCacheTTL:                10 * time.Second,
					DenyCacheTTL:                 10 * time.Second,
					RemoteKubeConfigFileOptional: true,
					AlwaysAllowPaths:             []string{"/healthz", "/readyz", "/livez"}, // note: this does not match /healthz/ or /healthz/*
					AlwaysAllowGroups:            []string{"system:masters"},
				},
				Logs: logs.NewOptions(),
			},
			expectedUsername: "flag",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  flagKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "default-scheduler",
						Plugins:       defaults.PluginsV1,
						PluginConfig:  defaults.PluginConfigsV1,
					},
				},
			},
		},
		{
			name: "overridden master",
			options: &Options{
				ComponentConfig: func() *kubeschedulerconfig.KubeSchedulerConfiguration {
					cfg, _ := latest.Default()
					cfg.ClientConnection.Kubeconfig = flagKubeconfig
					return cfg
				}(),
				Master: insecureserver.URL,
				SecureServing: (&apiserveroptions.SecureServingOptions{
					ServerCert: apiserveroptions.GeneratableKeyCert{
						CertDirectory: "/a/b/c",
						PairName:      "kube-scheduler",
					},
					HTTP2MaxStreamsPerConnection: 47,
				}).WithLoopback(),
				Authentication: &apiserveroptions.DelegatingAuthenticationOptions{
					CacheTTL: 10 * time.Second,
					RequestHeader: apiserveroptions.RequestHeaderAuthenticationOptions{
						UsernameHeaders:     []string{"x-remote-user"},
						GroupHeaders:        []string{"x-remote-group"},
						ExtraHeaderPrefixes: []string{"x-remote-extra-"},
					},
					RemoteKubeConfigFileOptional: true,
				},
				Authorization: &apiserveroptions.DelegatingAuthorizationOptions{
					AllowCacheTTL:                10 * time.Second,
					DenyCacheTTL:                 10 * time.Second,
					RemoteKubeConfigFileOptional: true,
					AlwaysAllowPaths:             []string{"/healthz", "/readyz", "/livez"}, // note: this does not match /healthz/ or /healthz/*
					AlwaysAllowGroups:            []string{"system:masters"},
				},
				Logs: logs.NewOptions(),
			},
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  flagKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "default-scheduler",
						Plugins:       defaults.PluginsV1,
						PluginConfig:  defaults.PluginConfigsV1,
					},
				},
			},
			expectedUsername: "none, http",
		},
		{
			name: "plugin config",
			options: &Options{
				ConfigFile: pluginConfigFile,
				Logs:       logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "default-scheduler",
						Plugins: &kubeschedulerconfig.Plugins{
							Reserve: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
									{Name: "bar"},
								},
								Disabled: []kubeschedulerconfig.Plugin{
									{Name: names.VolumeBinding},
								},
							},
							PreBind: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
								},
								Disabled: []kubeschedulerconfig.Plugin{
									{Name: names.VolumeBinding},
								},
							},
							MultiPoint: defaults.PluginsV1.MultiPoint,
						},
						PluginConfig: []kubeschedulerconfig.PluginConfig{
							{
								Name: "InterPodAffinity",
								Args: &kubeschedulerconfig.InterPodAffinityArgs{
									HardPodAffinityWeight: 2,
								},
							},
							{
								Name: "foo",
								Args: &runtime.Unknown{
									Raw:         []byte(`{"bar":"baz"}`),
									ContentType: "application/json",
								},
							},
							{
								Name: "DefaultPreemption",
								Args: &kubeschedulerconfig.DefaultPreemptionArgs{
									MinCandidateNodesPercentage: 10,
									MinCandidateNodesAbsolute:   100,
								},
							},
							{
								Name: "NodeAffinity",
								Args: &kubeschedulerconfig.NodeAffinityArgs{},
							},
							{
								Name: "NodeResourcesBalancedAllocation",
								Args: &kubeschedulerconfig.NodeResourcesBalancedAllocationArgs{
									Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
								},
							},
							{
								Name: "NodeResourcesFit",
								Args: &kubeschedulerconfig.NodeResourcesFitArgs{
									ScoringStrategy: &kubeschedulerconfig.ScoringStrategy{
										Type:      kubeschedulerconfig.LeastAllocated,
										Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
									},
								},
							},
							{
								Name: "PodTopologySpread",
								Args: &kubeschedulerconfig.PodTopologySpreadArgs{
									DefaultingType: kubeschedulerconfig.SystemDefaulting,
								},
							},
							{
								Name: "VolumeBinding",
								Args: &kubeschedulerconfig.VolumeBindingArgs{
									BindTimeoutSeconds: 600,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "v1beta3 plugin config",
			options: &Options{
				ConfigFile: v1beta3PluginConfigFile,
				Logs:       logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta3.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "default-scheduler",
						Plugins: &kubeschedulerconfig.Plugins{
							QueueSort:  defaults.PluginsV1beta3.QueueSort,
							PreFilter:  defaults.PluginsV1beta3.PreFilter,
							Filter:     defaults.PluginsV1beta3.Filter,
							PostFilter: defaults.PluginsV1beta3.PostFilter,
							PreScore:   defaults.PluginsV1beta3.PreScore,
							Score:      defaults.PluginsV1beta3.Score,
							Reserve: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
									{Name: "bar"},
								},
								Disabled: []kubeschedulerconfig.Plugin{
									{Name: "VolumeBinding"},
								},
							},
							PreBind: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
								},
								Disabled: []kubeschedulerconfig.Plugin{
									{Name: "VolumeBinding"},
								},
							},
							Bind:       defaults.PluginsV1beta3.Bind,
							MultiPoint: defaults.PluginsV1beta3.MultiPoint,
						},
						PluginConfig: []kubeschedulerconfig.PluginConfig{
							{
								Name: "InterPodAffinity",
								Args: &kubeschedulerconfig.InterPodAffinityArgs{
									HardPodAffinityWeight: 2,
								},
							},
							{
								Name: "foo",
								Args: &runtime.Unknown{
									Raw:         []byte(`{"bar":"baz"}`),
									ContentType: "application/json",
								},
							},
							{
								Name: "DefaultPreemption",
								Args: &kubeschedulerconfig.DefaultPreemptionArgs{
									MinCandidateNodesPercentage: 10,
									MinCandidateNodesAbsolute:   100,
								},
							},
							{
								Name: "NodeAffinity",
								Args: &kubeschedulerconfig.NodeAffinityArgs{},
							},
							{
								Name: "NodeResourcesBalancedAllocation",
								Args: &kubeschedulerconfig.NodeResourcesBalancedAllocationArgs{
									Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
								},
							},
							{
								Name: "NodeResourcesFit",
								Args: &kubeschedulerconfig.NodeResourcesFitArgs{
									ScoringStrategy: &kubeschedulerconfig.ScoringStrategy{
										Type:      kubeschedulerconfig.LeastAllocated,
										Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
									},
								},
							},
							{
								Name: "PodTopologySpread",
								Args: &kubeschedulerconfig.PodTopologySpreadArgs{
									DefaultingType: kubeschedulerconfig.SystemDefaulting,
								},
							},
							{
								Name: "VolumeBinding",
								Args: &kubeschedulerconfig.VolumeBindingArgs{
									BindTimeoutSeconds: 600,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "v1beta2 plugin config",
			options: &Options{
				ConfigFile: v1beta2PluginConfigFile,
				Logs:       logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta2.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "default-scheduler",
						Plugins: &kubeschedulerconfig.Plugins{
							QueueSort:  defaults.PluginsV1beta2.QueueSort,
							PreFilter:  defaults.PluginsV1beta2.PreFilter,
							Filter:     defaults.PluginsV1beta2.Filter,
							PostFilter: defaults.PluginsV1beta2.PostFilter,
							PreScore:   defaults.PluginsV1beta2.PreScore,
							Score:      defaults.PluginsV1beta2.Score,
							Reserve: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
									{Name: "bar"},
								},
							},
							PreBind: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
								},
							},
							Bind: defaults.PluginsV1beta2.Bind,
						},
						PluginConfig: []kubeschedulerconfig.PluginConfig{
							{
								Name: "InterPodAffinity",
								Args: &kubeschedulerconfig.InterPodAffinityArgs{
									HardPodAffinityWeight: 2,
								},
							},
							{
								Name: "foo",
								Args: &runtime.Unknown{
									Raw:         []byte(`{"bar":"baz"}`),
									ContentType: "application/json",
								},
							},
							{
								Name: "DefaultPreemption",
								Args: &kubeschedulerconfig.DefaultPreemptionArgs{
									MinCandidateNodesPercentage: 10,
									MinCandidateNodesAbsolute:   100,
								},
							},
							{
								Name: "NodeAffinity",
								Args: &kubeschedulerconfig.NodeAffinityArgs{},
							},
							{
								Name: "NodeResourcesBalancedAllocation",
								Args: &kubeschedulerconfig.NodeResourcesBalancedAllocationArgs{
									Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
								},
							},
							{
								Name: "NodeResourcesFit",
								Args: &kubeschedulerconfig.NodeResourcesFitArgs{
									ScoringStrategy: &kubeschedulerconfig.ScoringStrategy{
										Type:      kubeschedulerconfig.LeastAllocated,
										Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
									},
								},
							},
							{
								Name: "PodTopologySpread",
								Args: &kubeschedulerconfig.PodTopologySpreadArgs{
									DefaultingType: kubeschedulerconfig.SystemDefaulting,
								},
							},
							{
								Name: "VolumeBinding",
								Args: &kubeschedulerconfig.VolumeBindingArgs{
									BindTimeoutSeconds: 600,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple profiles",
			options: &Options{
				ConfigFile: multiProfilesConfig,
				Logs:       logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "foo-profile",
						Plugins: &kubeschedulerconfig.Plugins{
							MultiPoint: defaults.PluginsV1.MultiPoint,
							Reserve: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
									{Name: names.VolumeBinding},
								},
								Disabled: []kubeschedulerconfig.Plugin{
									{Name: names.VolumeBinding},
								},
							},
						},
						PluginConfig: defaults.PluginConfigsV1,
					},
					{
						SchedulerName: "bar-profile",
						Plugins: &kubeschedulerconfig.Plugins{
							MultiPoint: defaults.PluginsV1.MultiPoint,
							PreBind: kubeschedulerconfig.PluginSet{
								Disabled: []kubeschedulerconfig.Plugin{
									{Name: names.VolumeBinding},
								},
							},
						},
						PluginConfig: []kubeschedulerconfig.PluginConfig{
							{
								Name: "foo",
							},
							{
								Name: "DefaultPreemption",
								Args: &kubeschedulerconfig.DefaultPreemptionArgs{
									MinCandidateNodesPercentage: 10,
									MinCandidateNodesAbsolute:   100,
								},
							},
							{
								Name: "InterPodAffinity",
								Args: &kubeschedulerconfig.InterPodAffinityArgs{
									HardPodAffinityWeight: 1,
								},
							},
							{
								Name: "NodeAffinity",
								Args: &kubeschedulerconfig.NodeAffinityArgs{},
							},
							{
								Name: "NodeResourcesBalancedAllocation",
								Args: &kubeschedulerconfig.NodeResourcesBalancedAllocationArgs{
									Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
								},
							},
							{
								Name: "NodeResourcesFit",
								Args: &kubeschedulerconfig.NodeResourcesFitArgs{
									ScoringStrategy: &kubeschedulerconfig.ScoringStrategy{
										Type:      kubeschedulerconfig.LeastAllocated,
										Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
									},
								},
							},
							{
								Name: "PodTopologySpread",
								Args: &kubeschedulerconfig.PodTopologySpreadArgs{
									DefaultingType: kubeschedulerconfig.SystemDefaulting,
								},
							},
							{
								Name: "VolumeBinding",
								Args: &kubeschedulerconfig.VolumeBindingArgs{
									BindTimeoutSeconds: 600,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "v1beta3 multiple profiles",
			options: &Options{
				ConfigFile: v1beta3MultiProfilesConfig,
				Logs:       logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta3.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "foo-profile",
						Plugins: &kubeschedulerconfig.Plugins{
							MultiPoint: defaults.PluginsV1beta3.MultiPoint,
							Reserve: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
									{Name: names.VolumeBinding},
								},
								Disabled: []kubeschedulerconfig.Plugin{
									{Name: names.VolumeBinding},
								},
							},
						},
						PluginConfig: defaults.PluginConfigsV1beta3,
					},
					{
						SchedulerName: "bar-profile",
						Plugins: &kubeschedulerconfig.Plugins{
							MultiPoint: defaults.PluginsV1beta3.MultiPoint,
							PreBind: kubeschedulerconfig.PluginSet{
								Disabled: []kubeschedulerconfig.Plugin{
									{Name: names.VolumeBinding},
								},
							},
						},
						PluginConfig: []kubeschedulerconfig.PluginConfig{
							{
								Name: "foo",
							},
							{
								Name: "DefaultPreemption",
								Args: &kubeschedulerconfig.DefaultPreemptionArgs{
									MinCandidateNodesPercentage: 10,
									MinCandidateNodesAbsolute:   100,
								},
							},
							{
								Name: "InterPodAffinity",
								Args: &kubeschedulerconfig.InterPodAffinityArgs{
									HardPodAffinityWeight: 1,
								},
							},
							{
								Name: "NodeAffinity",
								Args: &kubeschedulerconfig.NodeAffinityArgs{},
							},
							{
								Name: "NodeResourcesBalancedAllocation",
								Args: &kubeschedulerconfig.NodeResourcesBalancedAllocationArgs{
									Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
								},
							},
							{
								Name: "NodeResourcesFit",
								Args: &kubeschedulerconfig.NodeResourcesFitArgs{
									ScoringStrategy: &kubeschedulerconfig.ScoringStrategy{
										Type:      kubeschedulerconfig.LeastAllocated,
										Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
									},
								},
							},
							{
								Name: "PodTopologySpread",
								Args: &kubeschedulerconfig.PodTopologySpreadArgs{
									DefaultingType: kubeschedulerconfig.SystemDefaulting,
								},
							},
							{
								Name: "VolumeBinding",
								Args: &kubeschedulerconfig.VolumeBindingArgs{
									BindTimeoutSeconds: 600,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "v1beta2 multiple profiles",
			options: &Options{
				ConfigFile: v1beta2MultiProfilesConfig,
				Logs:       logs.NewOptions(),
			},
			expectedUsername: "config",
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta2.SchemeGroupVersion.String(),
				},
				Parallelism: 16,
				DebuggingConfiguration: componentbaseconfig.DebuggingConfiguration{
					EnableProfiling:           true,
					EnableContentionProfiling: true,
				},
				LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:       true,
					LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
					RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
					ResourceLock:      "leases",
					ResourceNamespace: "kube-system",
					ResourceName:      "kube-scheduler",
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				PercentageOfNodesToScore: defaultPercentageOfNodesToScore,
				PodInitialBackoffSeconds: defaultPodInitialBackoffSeconds,
				PodMaxBackoffSeconds:     defaultPodMaxBackoffSeconds,
				Profiles: []kubeschedulerconfig.KubeSchedulerProfile{
					{
						SchedulerName: "foo-profile",
						Plugins: &kubeschedulerconfig.Plugins{
							QueueSort:  defaults.PluginsV1beta2.QueueSort,
							PreFilter:  defaults.PluginsV1beta2.PreFilter,
							Filter:     defaults.PluginsV1beta2.Filter,
							PostFilter: defaults.PluginsV1beta2.PostFilter,
							PreScore:   defaults.PluginsV1beta2.PreScore,
							Score:      defaults.PluginsV1beta2.Score,
							Bind:       defaults.PluginsV1beta2.Bind,
							PreBind:    defaults.PluginsV1beta2.PreBind,
							Reserve: kubeschedulerconfig.PluginSet{
								Enabled: []kubeschedulerconfig.Plugin{
									{Name: "foo"},
									{Name: names.VolumeBinding},
								},
							},
						},
						PluginConfig: defaults.PluginConfigsV1beta2,
					},
					{
						SchedulerName: "bar-profile",
						Plugins: &kubeschedulerconfig.Plugins{
							QueueSort:  defaults.PluginsV1beta2.QueueSort,
							PreFilter:  defaults.PluginsV1beta2.PreFilter,
							Filter:     defaults.PluginsV1beta2.Filter,
							PostFilter: defaults.PluginsV1beta2.PostFilter,
							PreScore:   defaults.PluginsV1beta2.PreScore,
							Score:      defaults.PluginsV1beta2.Score,
							Bind:       defaults.PluginsV1beta2.Bind,
							Reserve:    defaults.PluginsV1beta2.Reserve,
						},
						PluginConfig: []kubeschedulerconfig.PluginConfig{
							{
								Name: "foo",
							},
							{
								Name: "DefaultPreemption",
								Args: &kubeschedulerconfig.DefaultPreemptionArgs{
									MinCandidateNodesPercentage: 10,
									MinCandidateNodesAbsolute:   100,
								},
							},
							{
								Name: "InterPodAffinity",
								Args: &kubeschedulerconfig.InterPodAffinityArgs{
									HardPodAffinityWeight: 1,
								},
							},
							{
								Name: "NodeAffinity",
								Args: &kubeschedulerconfig.NodeAffinityArgs{},
							},
							{
								Name: "NodeResourcesBalancedAllocation",
								Args: &kubeschedulerconfig.NodeResourcesBalancedAllocationArgs{
									Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
								},
							},
							{
								Name: "NodeResourcesFit",
								Args: &kubeschedulerconfig.NodeResourcesFitArgs{
									ScoringStrategy: &kubeschedulerconfig.ScoringStrategy{
										Type:      kubeschedulerconfig.LeastAllocated,
										Resources: []kubeschedulerconfig.ResourceSpec{{Name: "cpu", Weight: 1}, {Name: "memory", Weight: 1}},
									},
								},
							},
							{
								Name: "PodTopologySpread",
								Args: &kubeschedulerconfig.PodTopologySpreadArgs{
									DefaultingType: kubeschedulerconfig.SystemDefaulting,
								},
							},
							{
								Name: "VolumeBinding",
								Args: &kubeschedulerconfig.VolumeBindingArgs{
									BindTimeoutSeconds: 600,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "no config",
			options: &Options{
				Logs: logs.NewOptions(),
			},
			expectedError: "no configuration has been provided",
		},
		{
			name: "unknown field",
			options: &Options{
				ConfigFile: unknownFieldConfig,
				Logs:       logs.NewOptions(),
			},
			expectedError: `unknown field "foo"`,
			checkErrFn:    runtime.IsStrictDecodingError,
		},
		{
			name: "duplicate fields",
			options: &Options{
				ConfigFile: duplicateFieldConfig,
				Logs:       logs.NewOptions(),
			},
			expectedError: `key "leaderElect" already set`,
			checkErrFn:    runtime.IsStrictDecodingError,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.options.ComponentConfig == nil {
				if cfg, err := latest.Default(); err != nil {
					t.Fatal(err)
				} else {
					tc.options.ComponentConfig = cfg
				}
			}
			// create the config
			config, err := tc.options.Config()

			// handle errors
			if err != nil {
				if tc.expectedError != "" || tc.checkErrFn != nil {
					if tc.expectedError != "" {
						assert.Contains(t, err.Error(), tc.expectedError)
					}
					if tc.checkErrFn != nil {
						assert.True(t, tc.checkErrFn(err), "got error: %v", err)
					}
					return
				}
				t.Errorf("unexpected error to create a config: %v", err)
				return
			}

			if _, err := encodeConfig(&config.ComponentConfig); err != nil {
				t.Errorf("unexpected error in encodeConfig: %v", err)
			}

			if diff := cmp.Diff(tc.expectedConfig, config.ComponentConfig); diff != "" {
				t.Errorf("incorrect config (-want,+got):\n%s", diff)
			}

			// ensure we have a client
			if config.Client == nil {
				t.Error("unexpected nil client")
				return
			}

			// test the client talks to the endpoint we expect with the credentials we expect
			username = ""
			_, err = config.Client.Discovery().RESTClient().Get().AbsPath("/").DoRaw(context.TODO())
			if err != nil {
				t.Error(err)
				return
			}
			if username != tc.expectedUsername {
				t.Errorf("expected server call with user %q, got %q", tc.expectedUsername, username)
			}
		})
	}
}
