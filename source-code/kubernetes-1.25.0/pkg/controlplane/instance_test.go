/*
Copyright 2014 The Kubernetes Authors.

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

package controlplane

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	autoscalingapiv2beta1 "k8s.io/api/autoscaling/v2beta1"
	autoscalingapiv2beta2 "k8s.io/api/autoscaling/v2beta2"
	batchapiv1beta1 "k8s.io/api/batch/v1beta1"
	certificatesapiv1beta1 "k8s.io/api/certificates/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	nodev1beta1 "k8s.io/api/node/v1beta1"
	policyapiv1beta1 "k8s.io/api/policy/v1beta1"
	storageapiv1beta1 "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/server/resourceconfig"
	serverstorage "k8s.io/apiserver/pkg/server/storage"
	etcd3testing "k8s.io/apiserver/pkg/storage/etcd3/testing"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	restclient "k8s.io/client-go/rest"
	kubeversion "k8s.io/component-base/version"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	flowcontrolv1beta2 "k8s.io/kubernetes/pkg/apis/flowcontrol/v1beta2"
	"k8s.io/kubernetes/pkg/controlplane/reconcilers"
	"k8s.io/kubernetes/pkg/controlplane/storageversionhashdata"
	"k8s.io/kubernetes/pkg/kubeapiserver"
	kubeletclient "k8s.io/kubernetes/pkg/kubelet/client"
	certificatesrest "k8s.io/kubernetes/pkg/registry/certificates/rest"
	corerest "k8s.io/kubernetes/pkg/registry/core/rest"
	"k8s.io/kubernetes/pkg/registry/registrytest"
	netutils "k8s.io/utils/net"

	"github.com/stretchr/testify/assert"
)

// setUp is a convenience function for setting up for (most) tests.
func setUp(t *testing.T) (*etcd3testing.EtcdTestServer, Config, *assert.Assertions) {
	server, storageConfig := etcd3testing.NewUnsecuredEtcd3TestClientServer(t)

	config := &Config{
		GenericConfig: genericapiserver.NewConfig(legacyscheme.Codecs),
		ExtraConfig: ExtraConfig{
			APIResourceConfigSource: DefaultAPIResourceConfigSource(),
			APIServerServicePort:    443,
			MasterCount:             1,
			EndpointReconcilerType:  reconcilers.MasterCountReconcilerType,
			ServiceIPRange:          net.IPNet{IP: netutils.ParseIPSloppy("10.0.0.0"), Mask: net.CIDRMask(24, 32)},
		},
	}

	storageFactoryConfig := kubeapiserver.NewStorageFactoryConfig()
	resourceEncoding := resourceconfig.MergeResourceEncodingConfigs(storageFactoryConfig.DefaultResourceEncoding, storageFactoryConfig.ResourceEncodingOverrides)
	storageFactory := serverstorage.NewDefaultStorageFactory(*storageConfig, "application/vnd.kubernetes.protobuf", storageFactoryConfig.Serializer, resourceEncoding, DefaultAPIResourceConfigSource(), nil)

	etcdOptions := options.NewEtcdOptions(storageConfig)
	// unit tests don't need watch cache and it leaks lots of goroutines with etcd testing functions during unit tests
	etcdOptions.EnableWatchCache = false
	err := etcdOptions.ApplyWithStorageFactoryTo(storageFactory, config.GenericConfig)
	if err != nil {
		t.Fatal(err)
	}

	kubeVersion := kubeversion.Get()
	config.GenericConfig.Authorization.Authorizer = authorizerfactory.NewAlwaysAllowAuthorizer()
	config.GenericConfig.Version = &kubeVersion
	config.ExtraConfig.StorageFactory = storageFactory
	config.GenericConfig.LoopbackClientConfig = &restclient.Config{APIPath: "/api", ContentConfig: restclient.ContentConfig{NegotiatedSerializer: legacyscheme.Codecs}}
	config.GenericConfig.PublicAddress = netutils.ParseIPSloppy("192.168.10.4")
	config.GenericConfig.LegacyAPIGroupPrefixes = sets.NewString("/api")
	config.ExtraConfig.KubeletClientConfig = kubeletclient.KubeletClientConfig{Port: 10250}
	config.ExtraConfig.ProxyTransport = utilnet.SetTransportDefaults(&http.Transport{
		DialContext:     func(ctx context.Context, network, addr string) (net.Conn, error) { return nil, nil },
		TLSClientConfig: &tls.Config{},
	})

	// set fake SecureServingInfo because the listener port is needed for the kubernetes service
	config.GenericConfig.SecureServing = &genericapiserver.SecureServingInfo{Listener: fakeLocalhost443Listener{}}

	clientset, err := kubernetes.NewForConfig(config.GenericConfig.LoopbackClientConfig)
	if err != nil {
		t.Fatalf("unable to create client set due to %v", err)
	}
	config.ExtraConfig.VersionedInformers = informers.NewSharedInformerFactory(clientset, config.GenericConfig.LoopbackClientConfig.Timeout)

	return server, *config, assert.New(t)
}

type fakeLocalhost443Listener struct{}

func (fakeLocalhost443Listener) Accept() (net.Conn, error) {
	return nil, nil
}

func (fakeLocalhost443Listener) Close() error {
	return nil
}

func (fakeLocalhost443Listener) Addr() net.Addr {
	return &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 443,
	}
}

// TestLegacyRestStorageStrategies ensures that all Storage objects which are using the generic registry Store have
// their various strategies properly wired up. This surfaced as a bug where strategies defined Export functions, but
// they were never used outside of unit tests because the export strategies were not assigned inside the Store.
func TestLegacyRestStorageStrategies(t *testing.T) {
	_, etcdserver, apiserverCfg, _ := newInstance(t)
	defer etcdserver.Terminate(t)

	storageProvider := corerest.LegacyRESTStorageProvider{
		StorageFactory:       apiserverCfg.ExtraConfig.StorageFactory,
		ProxyTransport:       apiserverCfg.ExtraConfig.ProxyTransport,
		KubeletClientConfig:  apiserverCfg.ExtraConfig.KubeletClientConfig,
		EventTTL:             apiserverCfg.ExtraConfig.EventTTL,
		ServiceIPRange:       apiserverCfg.ExtraConfig.ServiceIPRange,
		ServiceNodePortRange: apiserverCfg.ExtraConfig.ServiceNodePortRange,
		LoopbackClientConfig: apiserverCfg.GenericConfig.LoopbackClientConfig,
	}

	_, apiGroupInfo, err := storageProvider.NewLegacyRESTStorage(serverstorage.NewResourceConfig(), apiserverCfg.GenericConfig.RESTOptionsGetter)
	if err != nil {
		t.Errorf("failed to create legacy REST storage: %v", err)
	}

	strategyErrors := registrytest.ValidateStorageStrategies(apiGroupInfo.VersionedResourcesStorageMap["v1"])
	for _, err := range strategyErrors {
		t.Error(err)
	}
}

func TestCertificatesRestStorageStrategies(t *testing.T) {
	_, etcdserver, apiserverCfg, _ := newInstance(t)
	defer etcdserver.Terminate(t)

	certStorageProvider := certificatesrest.RESTStorageProvider{}
	apiGroupInfo, err := certStorageProvider.NewRESTStorage(apiserverCfg.ExtraConfig.APIResourceConfigSource, apiserverCfg.GenericConfig.RESTOptionsGetter)
	if err != nil {
		t.Fatalf("unexpected error from REST storage: %v", err)
	}

	strategyErrors := registrytest.ValidateStorageStrategies(
		apiGroupInfo.VersionedResourcesStorageMap[certificatesapiv1beta1.SchemeGroupVersion.Version])
	for _, err := range strategyErrors {
		t.Error(err)
	}
}

func newInstance(t *testing.T) (*Instance, *etcd3testing.EtcdTestServer, Config, *assert.Assertions) {
	etcdserver, config, assert := setUp(t)

	apiserver, err := config.Complete().New(genericapiserver.NewEmptyDelegate())
	if err != nil {
		t.Fatalf("Error in bringing up the master: %v", err)
	}

	return apiserver, etcdserver, config, assert
}

// TestVersion tests /version
func TestVersion(t *testing.T) {
	s, etcdserver, _, _ := newInstance(t)
	defer etcdserver.Terminate(t)

	req, _ := http.NewRequest("GET", "/version", nil)
	resp := httptest.NewRecorder()
	s.GenericAPIServer.Handler.ServeHTTP(resp, req)
	if resp.Code != 200 {
		t.Fatalf("expected http 200, got: %d", resp.Code)
	}

	var info version.Info
	err := json.NewDecoder(resp.Body).Decode(&info)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(kubeversion.Get(), info) {
		t.Errorf("Expected %#v, Got %#v", kubeversion.Get(), info)
	}
}

func makeNodeList(nodes []string, nodeResources apiv1.NodeResources) *apiv1.NodeList {
	list := apiv1.NodeList{
		Items: make([]apiv1.Node, len(nodes)),
	}
	for i := range nodes {
		list.Items[i].Name = nodes[i]
		list.Items[i].Status.Capacity = nodeResources.Capacity
	}
	return &list
}

// TestGetNodeAddresses verifies that proper results are returned
// when requesting node addresses.
func TestGetNodeAddresses(t *testing.T) {
	assert := assert.New(t)

	fakeNodeClient := fake.NewSimpleClientset(makeNodeList([]string{"node1", "node2"}, apiv1.NodeResources{})).CoreV1().Nodes()
	addressProvider := nodeAddressProvider{fakeNodeClient}

	// Fail case (no addresses associated with nodes)
	addrs, err := addressProvider.externalAddresses()

	assert.Error(err, "addresses should have caused an error as there are no addresses.")
	assert.Equal([]string(nil), addrs)

	// Pass case with External type IP
	nodes, _ := fakeNodeClient.List(context.TODO(), metav1.ListOptions{})
	for index := range nodes.Items {
		nodes.Items[index].Status.Addresses = []apiv1.NodeAddress{{Type: apiv1.NodeExternalIP, Address: "127.0.0.1"}}
		fakeNodeClient.Update(context.TODO(), &nodes.Items[index], metav1.UpdateOptions{})
	}
	addrs, err = addressProvider.externalAddresses()
	assert.NoError(err, "addresses should not have returned an error.")
	assert.Equal([]string{"127.0.0.1", "127.0.0.1"}, addrs)
}

func TestGetNodeAddressesWithOnlySomeExternalIP(t *testing.T) {
	assert := assert.New(t)

	fakeNodeClient := fake.NewSimpleClientset(makeNodeList([]string{"node1", "node2", "node3"}, apiv1.NodeResources{})).CoreV1().Nodes()
	addressProvider := nodeAddressProvider{fakeNodeClient}

	// Pass case with 1 External type IP (index == 1) and nodes (indexes 0 & 2) have no External IP.
	nodes, _ := fakeNodeClient.List(context.TODO(), metav1.ListOptions{})
	nodes.Items[1].Status.Addresses = []apiv1.NodeAddress{{Type: apiv1.NodeExternalIP, Address: "127.0.0.1"}}
	fakeNodeClient.Update(context.TODO(), &nodes.Items[1], metav1.UpdateOptions{})

	addrs, err := addressProvider.externalAddresses()
	assert.NoError(err, "addresses should not have returned an error.")
	assert.Equal([]string{"127.0.0.1"}, addrs)
}

func decodeResponse(resp *http.Response, obj interface{}) error {
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, obj); err != nil {
		return err
	}
	return nil
}

// Because we need to be backwards compatible with release 1.1, at endpoints
// that exist in release 1.1, the responses should have empty APIVersion.
func TestAPIVersionOfDiscoveryEndpoints(t *testing.T) {
	apiserver, etcdserver, _, assert := newInstance(t)
	defer etcdserver.Terminate(t)

	server := httptest.NewServer(apiserver.GenericAPIServer.Handler.GoRestfulContainer.ServeMux)

	// /api exists in release-1.1
	resp, err := http.Get(server.URL + "/api")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	apiVersions := metav1.APIVersions{}
	assert.NoError(decodeResponse(resp, &apiVersions))
	assert.Equal(apiVersions.APIVersion, "")

	// /api/v1 exists in release-1.1
	resp, err = http.Get(server.URL + "/api/v1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	resourceList := metav1.APIResourceList{}
	assert.NoError(decodeResponse(resp, &resourceList))
	assert.Equal(resourceList.APIVersion, "")

	// /apis exists in release-1.1
	resp, err = http.Get(server.URL + "/apis")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	groupList := metav1.APIGroupList{}
	assert.NoError(decodeResponse(resp, &groupList))
	assert.Equal(groupList.APIVersion, "")

	// /apis/autoscaling doesn't exist in release-1.1, so the APIVersion field
	// should be non-empty in the results returned by the server.
	resp, err = http.Get(server.URL + "/apis/autoscaling")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	group := metav1.APIGroup{}
	assert.NoError(decodeResponse(resp, &group))
	assert.Equal(group.APIVersion, "v1")

	// apis/autoscaling/v1 doesn't exist in release-1.1, so the APIVersion field
	// should be non-empty in the results returned by the server.

	resp, err = http.Get(server.URL + "/apis/autoscaling/v1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	resourceList = metav1.APIResourceList{}
	assert.NoError(decodeResponse(resp, &resourceList))
	assert.Equal(resourceList.APIVersion, "v1")

}

// This test doesn't cover the apiregistration and apiextensions group, as they are installed by other apiservers.
func TestStorageVersionHashes(t *testing.T) {
	apiserver, etcdserver, _, _ := newInstance(t)
	defer etcdserver.Terminate(t)

	server := httptest.NewServer(apiserver.GenericAPIServer.Handler.GoRestfulContainer.ServeMux)

	c := &restclient.Config{
		Host:          server.URL,
		APIPath:       "/api",
		ContentConfig: restclient.ContentConfig{NegotiatedSerializer: legacyscheme.Codecs},
	}
	discover := discovery.NewDiscoveryClientForConfigOrDie(c)
	_, all, err := discover.ServerGroupsAndResources()
	if err != nil {
		t.Error(err)
	}
	var count int
	apiResources := sets.NewString()
	for _, g := range all {
		for _, r := range g.APIResources {
			apiResources.Insert(g.GroupVersion + "/" + r.Name)
			if strings.Contains(r.Name, "/") ||
				storageversionhashdata.NoStorageVersionHash.Has(g.GroupVersion+"/"+r.Name) {
				if r.StorageVersionHash != "" {
					t.Errorf("expect resource %s/%s to have empty storageVersionHash, got hash %q", g.GroupVersion, r.Name, r.StorageVersionHash)
				}
				continue
			}
			if r.StorageVersionHash == "" {
				t.Errorf("expect the storageVersionHash of %s/%s to exist", g.GroupVersion, r.Name)
				continue
			}
			// Uncomment the following line if you want to update storageversionhash/data.go
			// fmt.Printf("\"%s/%s\": \"%s\",\n", g.GroupVersion, r.Name, r.StorageVersionHash)
			expected := storageversionhashdata.GVRToStorageVersionHash[g.GroupVersion+"/"+r.Name]
			if r.StorageVersionHash != expected {
				t.Errorf("expect the storageVersionHash of %s/%s to be %q, got %q", g.GroupVersion, r.Name, expected, r.StorageVersionHash)
			}
			count++
		}
	}
	if count != len(storageversionhashdata.GVRToStorageVersionHash) {
		knownResources := sets.StringKeySet(storageversionhashdata.GVRToStorageVersionHash)
		t.Errorf("please remove the redundant entries from GVRToStorageVersionHash: %v", knownResources.Difference(apiResources).List())
	}
}

func TestNoAlphaVersionsEnabledByDefault(t *testing.T) {
	config := DefaultAPIResourceConfigSource()
	for gv, enable := range config.GroupVersionConfigs {
		if enable && strings.Contains(gv.Version, "alpha") {
			t.Errorf("Alpha API version %s enabled by default", gv.String())
		}
	}
}

func TestNoBetaVersionsEnabledByDefault(t *testing.T) {
	config := DefaultAPIResourceConfigSource()
	for gv, enable := range config.GroupVersionConfigs {
		if enable && strings.Contains(gv.Version, "beta") {
			t.Errorf("Beta API version %s enabled by default", gv.String())
		}
	}
}

func TestNewBetaResourcesEnabledByDefault(t *testing.T) {
	// legacyEnabledBetaResources is nearly a duplication from elsewhere.  This is intentional.  These types already have
	// GA equivalents available and should therefore never have a beta enabled by default again.
	legacyEnabledBetaResources := map[schema.GroupVersionResource]bool{
		autoscalingapiv2beta1.SchemeGroupVersion.WithResource("horizontalpodautoscalers"): true,
		autoscalingapiv2beta2.SchemeGroupVersion.WithResource("horizontalpodautoscalers"): true,
		batchapiv1beta1.SchemeGroupVersion.WithResource("cronjobs"):                       true,
		discoveryv1beta1.SchemeGroupVersion.WithResource("endpointslices"):                true,
		eventsv1beta1.SchemeGroupVersion.WithResource("events"):                           true,
		nodev1beta1.SchemeGroupVersion.WithResource("runtimeclasses"):                     true,
		policyapiv1beta1.SchemeGroupVersion.WithResource("poddisruptionbudgets"):          true,
		policyapiv1beta1.SchemeGroupVersion.WithResource("podsecuritypolicies"):           true,
		storageapiv1beta1.SchemeGroupVersion.WithResource("csinodes"):                     true,
		storageapiv1beta1.SchemeGroupVersion.WithResource("csistoragecapacities"):         true,
	}

	// legacyBetaResourcesWithoutStableEquivalents contains those groupresources that were enabled by default as beta
	// before we changed that policy and do not have stable versions. These resources are allowed to have additional
	// beta versions enabled by default.  Nothing new should be added here.  There are no future exceptions because there
	// are no more beta resources enabled by default.
	legacyBetaResourcesWithoutStableEquivalents := map[schema.GroupResource]bool{
		storageapiv1beta1.SchemeGroupVersion.WithResource("csistoragecapacities").GroupResource():         true,
		flowcontrolv1beta2.SchemeGroupVersion.WithResource("flowschemas").GroupResource():                 true,
		flowcontrolv1beta2.SchemeGroupVersion.WithResource("prioritylevelconfigurations").GroupResource(): true,
	}

	config := DefaultAPIResourceConfigSource()
	for gvr, enable := range config.ResourceConfigs {
		if !strings.Contains(gvr.Version, "beta") {
			continue // only check beta things
		}
		if !enable {
			continue // only check things that are enabled
		}
		if legacyEnabledBetaResources[gvr] {
			continue // this is a legacy beta resource
		}
		if legacyBetaResourcesWithoutStableEquivalents[gvr.GroupResource()] {
			continue // this is another beta of a legacy beta resource with no stable equivalent
		}
		t.Errorf("no new beta resources can be enabled by default, see https://github.com/kubernetes/enhancements/blob/0ad0fc8269165ca300d05ca51c7ce190a79976a5/keps/sig-architecture/3136-beta-apis-off-by-default/README.md: %v", gvr)
	}
}
