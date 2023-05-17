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

package main

import (
	"errors"
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	internalapi "k8s.io/cri-api/pkg/apis"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/events"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	_ "k8s.io/component-base/metrics/prometheus/restclient" // for client metric registration
	_ "k8s.io/component-base/metrics/prometheus/version"    // for version metric registration
	"k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/cluster/ports"
	cadvisortest "k8s.io/kubernetes/pkg/kubelet/cadvisor/testing"
	"k8s.io/kubernetes/pkg/kubelet/cm"
	"k8s.io/kubernetes/pkg/kubelet/cri/remote"
	fakeremote "k8s.io/kubernetes/pkg/kubelet/cri/remote/fake"
	"k8s.io/kubernetes/pkg/kubemark"
	fakeiptables "k8s.io/kubernetes/pkg/util/iptables/testing"
	fakesysctl "k8s.io/kubernetes/pkg/util/sysctl/testing"
	utiltaints "k8s.io/kubernetes/pkg/util/taints"
	fakeexec "k8s.io/utils/exec/testing"
)

type hollowNodeConfig struct {
	KubeconfigPath       string
	KubeletPort          int
	KubeletReadOnlyPort  int
	Morph                string
	NodeName             string
	ServerPort           int
	ContentType          string
	UseRealProxier       bool
	ProxierSyncPeriod    time.Duration
	ProxierMinSyncPeriod time.Duration
	NodeLabels           map[string]string
	RegisterWithTaints   []core.Taint
	MaxPods              int
	ExtendedResources    map[string]string
	UseHostImageService  bool
}

const (
	maxPods     = 110
	podsPerCore = 0
)

// TODO(#45650): Refactor hollow-node into hollow-kubelet and hollow-proxy
// and make the config driven.
var knownMorphs = sets.NewString("kubelet", "proxy")

func (c *hollowNodeConfig) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.KubeconfigPath, "kubeconfig", "/kubeconfig/kubeconfig", "Path to kubeconfig file.")
	fs.IntVar(&c.KubeletPort, "kubelet-port", ports.KubeletPort, "Port on which HollowKubelet should be listening.")
	fs.IntVar(&c.KubeletReadOnlyPort, "kubelet-read-only-port", ports.KubeletReadOnlyPort, "Read-only port on which Kubelet is listening.")
	fs.StringVar(&c.NodeName, "name", "fake-node", "Name of this Hollow Node.")
	fs.IntVar(&c.ServerPort, "api-server-port", 443, "Port on which API server is listening.")
	fs.StringVar(&c.Morph, "morph", "", fmt.Sprintf("Specifies into which Hollow component this binary should morph. Allowed values: %v", knownMorphs.List()))
	fs.StringVar(&c.ContentType, "kube-api-content-type", "application/vnd.kubernetes.protobuf", "ContentType of requests sent to apiserver.")
	fs.BoolVar(&c.UseRealProxier, "use-real-proxier", true, "Set to true if you want to use real proxier inside hollow-proxy.")
	fs.DurationVar(&c.ProxierSyncPeriod, "proxier-sync-period", 30*time.Second, "Period that proxy rules are refreshed in hollow-proxy.")
	fs.DurationVar(&c.ProxierMinSyncPeriod, "proxier-min-sync-period", 0, "Minimum period that proxy rules are refreshed in hollow-proxy.")
	bindableNodeLabels := cliflag.ConfigurationMap(c.NodeLabels)
	fs.Var(&bindableNodeLabels, "node-labels", "Additional node labels")
	fs.Var(utiltaints.NewTaintsVar(&c.RegisterWithTaints), "register-with-taints", "Register the node with the given list of taints (comma separated \"<key>=<value>:<effect>\"). No-op if register-node is false.")
	fs.IntVar(&c.MaxPods, "max-pods", maxPods, "Number of pods that can run on this Kubelet.")
	bindableExtendedResources := cliflag.ConfigurationMap(c.ExtendedResources)
	fs.Var(&bindableExtendedResources, "extended-resources", "Register the node with extended resources (comma separated \"<name>=<quantity>\")")
	fs.BoolVar(&c.UseHostImageService, "use-host-image-service", true, "Set to true if the hollow-kubelet should use the host image service. If set to false the fake image service will be used")
}

func (c *hollowNodeConfig) createClientConfigFromFile() (*restclient.Config, error) {
	clientConfig, err := clientcmd.LoadFromFile(c.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("error while loading kubeconfig from file %v: %v", c.KubeconfigPath, err)
	}
	config, err := clientcmd.NewDefaultClientConfig(*clientConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("error while creating kubeconfig: %v", err)
	}
	config.ContentType = c.ContentType
	config.QPS = 10
	config.Burst = 20
	return config, nil
}

func (c *hollowNodeConfig) createHollowKubeletOptions() *kubemark.HollowKubletOptions {
	return &kubemark.HollowKubletOptions{
		NodeName:            c.NodeName,
		KubeletPort:         c.KubeletPort,
		KubeletReadOnlyPort: c.KubeletReadOnlyPort,
		MaxPods:             c.MaxPods,
		PodsPerCore:         podsPerCore,
		NodeLabels:          c.NodeLabels,
		RegisterWithTaints:  c.RegisterWithTaints,
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	command := newHollowNodeCommand()

	// TODO: once we switch everything over to Cobra commands, we can go back to calling
	// cliflag.InitFlags() (by removing its pflag.Parse() call). For now, we have to set the
	// normalize func and add the go flag set by hand.
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	// cliflag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

// newControllerManagerCommand creates a *cobra.Command object with default parameters
func newHollowNodeCommand() *cobra.Command {
	s := &hollowNodeConfig{
		NodeLabels:        make(map[string]string),
		ExtendedResources: make(map[string]string),
	}

	cmd := &cobra.Command{
		Use:  "kubemark",
		Long: "kubemark",
		Run: func(cmd *cobra.Command, args []string) {
			verflag.PrintAndExitIfRequested()
			run(cmd, s)
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
	}
	s.addFlags(cmd.Flags())

	return cmd
}

func run(cmd *cobra.Command, config *hollowNodeConfig) {
	// To help debugging, immediately log version and print flags.
	klog.Infof("Version: %+v", version.Get())
	cliflag.PrintFlags(cmd.Flags())

	if !knownMorphs.Has(config.Morph) {
		klog.Fatalf("Unknown morph: %v. Allowed values: %v", config.Morph, knownMorphs.List())
	}

	// create a client to communicate with API server.
	clientConfig, err := config.createClientConfigFromFile()
	if err != nil {
		klog.Fatalf("Failed to create a ClientConfig: %v. Exiting.", err)
	}

	client, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		klog.Fatalf("Failed to create a ClientSet: %v. Exiting.", err)
	}

	if config.Morph == "kubelet" {
		f, c := kubemark.GetHollowKubeletConfig(config.createHollowKubeletOptions())

		heartbeatClientConfig := *clientConfig
		heartbeatClientConfig.Timeout = c.NodeStatusUpdateFrequency.Duration
		// The timeout is the minimum of the lease duration and status update frequency
		leaseTimeout := time.Duration(c.NodeLeaseDurationSeconds) * time.Second
		if heartbeatClientConfig.Timeout > leaseTimeout {
			heartbeatClientConfig.Timeout = leaseTimeout
		}

		heartbeatClientConfig.QPS = float32(-1)
		heartbeatClient, err := clientset.NewForConfig(&heartbeatClientConfig)
		if err != nil {
			klog.Fatalf("Failed to create a ClientSet: %v. Exiting.", err)
		}

		cadvisorInterface := &cadvisortest.Fake{
			NodeName: config.NodeName,
		}

		var containerManager cm.ContainerManager
		if config.ExtendedResources != nil {
			extendedResources := v1.ResourceList{}
			for k, v := range config.ExtendedResources {
				extendedResources[v1.ResourceName(k)] = resource.MustParse(v)
			}

			containerManager = cm.NewStubContainerManagerWithDevicePluginResource(extendedResources)
		} else {
			containerManager = cm.NewStubContainerManager()
		}

		endpoint, err := fakeremote.GenerateEndpoint()
		if err != nil {
			klog.Fatalf("Failed to generate fake endpoint %v.", err)
		}
		fakeRemoteRuntime := fakeremote.NewFakeRemoteRuntime()
		if err = fakeRemoteRuntime.Start(endpoint); err != nil {
			klog.Fatalf("Failed to start fake runtime %v.", err)
		}
		defer fakeRemoteRuntime.Stop()
		runtimeService, err := remote.NewRemoteRuntimeService(endpoint, 15*time.Second)
		if err != nil {
			klog.Fatalf("Failed to init runtime service %v.", err)
		}

		var imageService internalapi.ImageManagerService = fakeRemoteRuntime.ImageService
		if config.UseHostImageService {
			imageService, err = remote.NewRemoteImageService(f.RemoteImageEndpoint, 15*time.Second)
			if err != nil {
				klog.Fatalf("Failed to init image service %v.", err)
			}
		}

		hollowKubelet := kubemark.NewHollowKubelet(
			f, c,
			client,
			heartbeatClient,
			cadvisorInterface,
			imageService,
			runtimeService,
			containerManager,
		)
		hollowKubelet.Run()
	}

	if config.Morph == "proxy" {
		client, err := clientset.NewForConfig(clientConfig)
		if err != nil {
			klog.Fatalf("Failed to create API Server client: %v", err)
		}
		iptInterface := fakeiptables.NewFake()
		sysctl := fakesysctl.NewFake()
		execer := &fakeexec.FakeExec{
			LookPathFunc: func(_ string) (string, error) { return "", errors.New("fake execer") },
		}
		eventBroadcaster := events.NewBroadcaster(&events.EventSinkImpl{Interface: client.EventsV1()})
		recorder := eventBroadcaster.NewRecorder(legacyscheme.Scheme, "kube-proxy")

		hollowProxy, err := kubemark.NewHollowProxyOrDie(
			config.NodeName,
			client,
			client.CoreV1(),
			iptInterface,
			sysctl,
			execer,
			eventBroadcaster,
			recorder,
			config.UseRealProxier,
			config.ProxierSyncPeriod,
			config.ProxierMinSyncPeriod,
		)
		if err != nil {
			klog.Fatalf("Failed to create hollowProxy instance: %v", err)
		}
		hollowProxy.Run()
	}
}
