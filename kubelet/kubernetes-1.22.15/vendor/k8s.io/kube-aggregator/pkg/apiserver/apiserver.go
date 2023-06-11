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

package apiserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	genericfeatures "k8s.io/apiserver/pkg/features"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/egressselector"
	serverstorage "k8s.io/apiserver/pkg/server/storage"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/version"
	openapicommon "k8s.io/kube-openapi/pkg/common"

	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	v1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	v1helper "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1/helper"
	"k8s.io/kube-aggregator/pkg/apis/apiregistration/v1beta1"
	aggregatorscheme "k8s.io/kube-aggregator/pkg/apiserver/scheme"
	"k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	informers "k8s.io/kube-aggregator/pkg/client/informers/externalversions"
	listers "k8s.io/kube-aggregator/pkg/client/listers/apiregistration/v1"
	openapicontroller "k8s.io/kube-aggregator/pkg/controllers/openapi"
	openapiaggregator "k8s.io/kube-aggregator/pkg/controllers/openapi/aggregator"
	statuscontrollers "k8s.io/kube-aggregator/pkg/controllers/status"
	apiservicerest "k8s.io/kube-aggregator/pkg/registry/apiservice/rest"
)

func init() {
	// we need to add the options (like ListOptions) to empty v1
	metav1.AddToGroupVersion(aggregatorscheme.Scheme, schema.GroupVersion{Group: "", Version: "v1"})

	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	aggregatorscheme.Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

const (
	// legacyAPIServiceName is the fixed name of the only non-groupified API version
	legacyAPIServiceName = "v1."
	// StorageVersionPostStartHookName is the name of the storage version updater post start hook.
	StorageVersionPostStartHookName = "built-in-resources-storage-version-updater"
)

// ExtraConfig represents APIServices-specific configuration
type ExtraConfig struct {
	// ProxyClientCert/Key are the client cert used to identify this proxy. Backing APIServices use
	// this to confirm the proxy's identity
	ProxyClientCertFile string
	ProxyClientKeyFile  string

	// If present, the Dial method will be used for dialing out to delegate
	// apiservers.
	ProxyTransport *http.Transport

	// Mechanism by which the Aggregator will resolve services. Required.
	ServiceResolver ServiceResolver

	RejectForwardingRedirects bool
}

// Config represents the configuration needed to create an APIAggregator.
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// CompletedConfig same as Config, just to swap private object.
type CompletedConfig struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedConfig
}

type runnable interface {
	Run(stopCh <-chan struct{}) error
}

// preparedGenericAPIServer is a private wrapper that enforces a call of PrepareRun() before Run can be invoked.
type preparedAPIAggregator struct {
	*APIAggregator
	runnable runnable
}

// APIAggregator contains state for a Kubernetes cluster master/api server.
type APIAggregator struct {
	GenericAPIServer *genericapiserver.GenericAPIServer

	delegateHandler http.Handler

	// proxyCurrentCertKeyContent holds he client cert used to identify this proxy. Backing APIServices use this to confirm the proxy's identity
	proxyCurrentCertKeyContent certKeyFunc
	proxyTransport             *http.Transport

	// proxyHandlers are the proxy handlers that are currently registered, keyed by apiservice.name
	proxyHandlers map[string]*proxyHandler
	// handledGroups are the groups that already have routes
	handledGroups sets.String

	// lister is used to add group handling for /apis/<group> aggregator lookups based on
	// controller state
	lister listers.APIServiceLister

	// provided for easier embedding
	APIRegistrationInformers informers.SharedInformerFactory

	// Information needed to determine routing for the aggregator
	serviceResolver ServiceResolver

	// Enable swagger and/or OpenAPI if these configs are non-nil.
	openAPIConfig *openapicommon.Config

	// openAPIAggregationController downloads and merges OpenAPI specs.
	openAPIAggregationController *openapicontroller.AggregationController

	// egressSelector selects the proper egress dialer to communicate with the custom apiserver
	// overwrites proxyTransport dialer if not nil
	egressSelector *egressselector.EgressSelector

	// rejectForwardingRedirects is whether to allow to forward redirect response
	rejectForwardingRedirects bool
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		&cfg.ExtraConfig,
	}

	// the kube aggregator wires its own discovery mechanism
	// TODO eventually collapse this by extracting all of the discovery out
	c.GenericConfig.EnableDiscovery = false
	version := version.Get()
	c.GenericConfig.Version = &version

	return CompletedConfig{&c}
}

// NewWithDelegate returns a new instance of APIAggregator from the given config.
func (c completedConfig) NewWithDelegate(delegationTarget genericapiserver.DelegationTarget) (*APIAggregator, error) {
	genericServer, err := c.GenericConfig.New("kube-aggregator", delegationTarget)
	if err != nil {
		return nil, err
	}

	apiregistrationClient, err := clientset.NewForConfig(c.GenericConfig.LoopbackClientConfig)
	if err != nil {
		return nil, err
	}
	informerFactory := informers.NewSharedInformerFactory(
		apiregistrationClient,
		5*time.Minute, // this is effectively used as a refresh interval right now.  Might want to do something nicer later on.
	)

	s := &APIAggregator{
		GenericAPIServer:           genericServer,
		delegateHandler:            delegationTarget.UnprotectedHandler(),
		proxyTransport:             c.ExtraConfig.ProxyTransport,
		proxyHandlers:              map[string]*proxyHandler{},
		handledGroups:              sets.String{},
		lister:                     informerFactory.Apiregistration().V1().APIServices().Lister(),
		APIRegistrationInformers:   informerFactory,
		serviceResolver:            c.ExtraConfig.ServiceResolver,
		openAPIConfig:              c.GenericConfig.OpenAPIConfig,
		egressSelector:             c.GenericConfig.EgressSelector,
		proxyCurrentCertKeyContent: func() (bytes []byte, bytes2 []byte) { return nil, nil },
		rejectForwardingRedirects:  c.ExtraConfig.RejectForwardingRedirects,
	}

	// used later  to filter the served resource by those that have expired.
	resourceExpirationEvaluator, err := genericapiserver.NewResourceExpirationEvaluator(*c.GenericConfig.Version)
	if err != nil {
		return nil, err
	}

	apiGroupInfo := apiservicerest.NewRESTStorage(c.GenericConfig.MergedResourceConfig, c.GenericConfig.RESTOptionsGetter, resourceExpirationEvaluator.ShouldServeForVersion(1, 22))
	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	enabledVersions := sets.NewString()
	for v := range apiGroupInfo.VersionedResourcesStorageMap {
		enabledVersions.Insert(v)
	}
	if !enabledVersions.Has(v1.SchemeGroupVersion.Version) {
		return nil, fmt.Errorf("API group/version %s must be enabled", v1.SchemeGroupVersion.String())
	}

	apisHandler := &apisHandler{
		codecs:         aggregatorscheme.Codecs,
		lister:         s.lister,
		discoveryGroup: discoveryGroup(enabledVersions),
	}
	s.GenericAPIServer.Handler.NonGoRestfulMux.Handle("/apis", apisHandler)
	s.GenericAPIServer.Handler.NonGoRestfulMux.UnlistedHandle("/apis/", apisHandler)

	apiserviceRegistrationController := NewAPIServiceRegistrationController(informerFactory.Apiregistration().V1().APIServices(), s)
	if len(c.ExtraConfig.ProxyClientCertFile) > 0 && len(c.ExtraConfig.ProxyClientKeyFile) > 0 {
		aggregatorProxyCerts, err := dynamiccertificates.NewDynamicServingContentFromFiles("aggregator-proxy-cert", c.ExtraConfig.ProxyClientCertFile, c.ExtraConfig.ProxyClientKeyFile)
		if err != nil {
			return nil, err
		}
		if err := aggregatorProxyCerts.RunOnce(); err != nil {
			return nil, err
		}
		aggregatorProxyCerts.AddListener(apiserviceRegistrationController)
		s.proxyCurrentCertKeyContent = aggregatorProxyCerts.CurrentCertKeyContent

		s.GenericAPIServer.AddPostStartHookOrDie("aggregator-reload-proxy-client-cert", func(context genericapiserver.PostStartHookContext) error {
			go aggregatorProxyCerts.Run(1, context.StopCh)
			return nil
		})
	}

	availableController, err := statuscontrollers.NewAvailableConditionController(
		informerFactory.Apiregistration().V1().APIServices(),
		c.GenericConfig.SharedInformerFactory.Core().V1().Services(),
		c.GenericConfig.SharedInformerFactory.Core().V1().Endpoints(),
		apiregistrationClient.ApiregistrationV1(),
		c.ExtraConfig.ProxyTransport,
		(func() ([]byte, []byte))(s.proxyCurrentCertKeyContent),
		s.serviceResolver,
		c.GenericConfig.EgressSelector,
	)
	if err != nil {
		return nil, err
	}

	s.GenericAPIServer.AddPostStartHookOrDie("start-kube-aggregator-informers", func(context genericapiserver.PostStartHookContext) error {
		informerFactory.Start(context.StopCh)
		c.GenericConfig.SharedInformerFactory.Start(context.StopCh)
		return nil
	})
	s.GenericAPIServer.AddPostStartHookOrDie("apiservice-registration-controller", func(context genericapiserver.PostStartHookContext) error {
		handlerSyncedCh := make(chan struct{})
		go apiserviceRegistrationController.Run(context.StopCh, handlerSyncedCh)
		select {
		case <-context.StopCh:
		case <-handlerSyncedCh:
		}

		return nil
	})
	s.GenericAPIServer.AddPostStartHookOrDie("apiservice-status-available-controller", func(context genericapiserver.PostStartHookContext) error {
		// if we end up blocking for long periods of time, we may need to increase threadiness.
		go availableController.Run(5, context.StopCh)
		return nil
	})

	if utilfeature.DefaultFeatureGate.Enabled(genericfeatures.StorageVersionAPI) &&
		utilfeature.DefaultFeatureGate.Enabled(genericfeatures.APIServerIdentity) {
		// Spawn a goroutine in aggregator apiserver to update storage version for
		// all built-in resources
		s.GenericAPIServer.AddPostStartHookOrDie(StorageVersionPostStartHookName, func(hookContext genericapiserver.PostStartHookContext) error {
			// Wait for apiserver-identity to exist first before updating storage
			// versions, to avoid storage version GC accidentally garbage-collecting
			// storage versions.
			kubeClient, err := kubernetes.NewForConfig(hookContext.LoopbackClientConfig)
			if err != nil {
				return err
			}
			if err := wait.PollImmediateUntil(100*time.Millisecond, func() (bool, error) {
				_, err := kubeClient.CoordinationV1().Leases(metav1.NamespaceSystem).Get(
					context.TODO(), s.GenericAPIServer.APIServerID, metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				if err != nil {
					return false, err
				}
				return true, nil
			}, hookContext.StopCh); err != nil {
				return fmt.Errorf("failed to wait for apiserver-identity lease %s to be created: %v",
					s.GenericAPIServer.APIServerID, err)
			}
			// Technically an apiserver only needs to update storage version once during bootstrap.
			// Reconcile StorageVersion objects every 10 minutes will help in the case that the
			// StorageVersion objects get accidentally modified/deleted by a different agent. In that
			// case, the reconciliation ensures future storage migration still works. If nothing gets
			// changed, the reconciliation update is a noop and gets short-circuited by the apiserver,
			// therefore won't change the resource version and trigger storage migration.
			go wait.PollImmediateUntil(10*time.Minute, func() (bool, error) {
				// All apiservers (aggregator-apiserver, kube-apiserver, apiextensions-apiserver)
				// share the same generic apiserver config. The same StorageVersion manager is used
				// to register all built-in resources when the generic apiservers install APIs.
				s.GenericAPIServer.StorageVersionManager.UpdateStorageVersions(hookContext.LoopbackClientConfig, s.GenericAPIServer.APIServerID)
				return false, nil
			}, hookContext.StopCh)
			// Once the storage version updater finishes the first round of update,
			// the PostStartHook will return to unblock /healthz. The handler chain
			// won't block write requests anymore. Check every second since it's not
			// expensive.
			wait.PollImmediateUntil(1*time.Second, func() (bool, error) {
				return s.GenericAPIServer.StorageVersionManager.Completed(), nil
			}, hookContext.StopCh)
			return nil
		})
	}

	return s, nil
}

// PrepareRun prepares the aggregator to run, by setting up the OpenAPI spec and calling
// the generic PrepareRun.
func (s *APIAggregator) PrepareRun() (preparedAPIAggregator, error) {
	// add post start hook before generic PrepareRun in order to be before /healthz installation
	if s.openAPIConfig != nil {
		s.GenericAPIServer.AddPostStartHookOrDie("apiservice-openapi-controller", func(context genericapiserver.PostStartHookContext) error {
			go s.openAPIAggregationController.Run(context.StopCh)
			return nil
		})
	}

	prepared := s.GenericAPIServer.PrepareRun()

	// delay OpenAPI setup until the delegate had a chance to setup their OpenAPI handlers
	if s.openAPIConfig != nil {
		specDownloader := openapiaggregator.NewDownloader()
		openAPIAggregator, err := openapiaggregator.BuildAndRegisterAggregator(
			&specDownloader,
			s.GenericAPIServer.NextDelegate(),
			s.GenericAPIServer.Handler.GoRestfulContainer.RegisteredWebServices(),
			s.openAPIConfig,
			s.GenericAPIServer.Handler.NonGoRestfulMux)
		if err != nil {
			return preparedAPIAggregator{}, err
		}
		s.openAPIAggregationController = openapicontroller.NewAggregationController(&specDownloader, openAPIAggregator)
	}

	return preparedAPIAggregator{APIAggregator: s, runnable: prepared}, nil
}

func (s preparedAPIAggregator) Run(stopCh <-chan struct{}) error {
	return s.runnable.Run(stopCh)
}

// AddAPIService adds an API service.  It is not thread-safe, so only call it on one thread at a time please.
// It's a slow moving API, so its ok to run the controller on a single thread
func (s *APIAggregator) AddAPIService(apiService *v1.APIService) error {
	// if the proxyHandler already exists, it needs to be updated. The aggregation bits do not
	// since they are wired against listers because they require multiple resources to respond
	if proxyHandler, exists := s.proxyHandlers[apiService.Name]; exists {
		proxyHandler.updateAPIService(apiService)
		if s.openAPIAggregationController != nil {
			s.openAPIAggregationController.UpdateAPIService(proxyHandler, apiService)
		}
		return nil
	}

	proxyPath := "/apis/" + apiService.Spec.Group + "/" + apiService.Spec.Version
	// v1. is a special case for the legacy API.  It proxies to a wider set of endpoints.
	if apiService.Name == legacyAPIServiceName {
		proxyPath = "/api"
	}

	// register the proxy handler
	proxyHandler := &proxyHandler{
		localDelegate:              s.delegateHandler,
		proxyCurrentCertKeyContent: s.proxyCurrentCertKeyContent,
		proxyTransport:             s.proxyTransport,
		serviceResolver:            s.serviceResolver,
		egressSelector:             s.egressSelector,
		rejectForwardingRedirects:  s.rejectForwardingRedirects,
	}
	proxyHandler.updateAPIService(apiService)
	if s.openAPIAggregationController != nil {
		s.openAPIAggregationController.AddAPIService(proxyHandler, apiService)
	}
	s.proxyHandlers[apiService.Name] = proxyHandler
	s.GenericAPIServer.Handler.NonGoRestfulMux.Handle(proxyPath, proxyHandler)
	s.GenericAPIServer.Handler.NonGoRestfulMux.UnlistedHandlePrefix(proxyPath+"/", proxyHandler)

	// if we're dealing with the legacy group, we're done here
	if apiService.Name == legacyAPIServiceName {
		return nil
	}

	// if we've already registered the path with the handler, we don't want to do it again.
	if s.handledGroups.Has(apiService.Spec.Group) {
		return nil
	}

	// it's time to register the group aggregation endpoint
	groupPath := "/apis/" + apiService.Spec.Group
	groupDiscoveryHandler := &apiGroupHandler{
		codecs:    aggregatorscheme.Codecs,
		groupName: apiService.Spec.Group,
		lister:    s.lister,
		delegate:  s.delegateHandler,
	}
	// aggregation is protected
	s.GenericAPIServer.Handler.NonGoRestfulMux.Handle(groupPath, groupDiscoveryHandler)
	s.GenericAPIServer.Handler.NonGoRestfulMux.UnlistedHandle(groupPath+"/", groupDiscoveryHandler)
	s.handledGroups.Insert(apiService.Spec.Group)
	return nil
}

// RemoveAPIService removes the APIService from being handled.  It is not thread-safe, so only call it on one thread at a time please.
// It's a slow moving API, so it's ok to run the controller on a single thread.
func (s *APIAggregator) RemoveAPIService(apiServiceName string) {
	version := v1helper.APIServiceNameToGroupVersion(apiServiceName)

	proxyPath := "/apis/" + version.Group + "/" + version.Version
	// v1. is a special case for the legacy API.  It proxies to a wider set of endpoints.
	if apiServiceName == legacyAPIServiceName {
		proxyPath = "/api"
	}
	s.GenericAPIServer.Handler.NonGoRestfulMux.Unregister(proxyPath)
	s.GenericAPIServer.Handler.NonGoRestfulMux.Unregister(proxyPath + "/")
	if s.openAPIAggregationController != nil {
		s.openAPIAggregationController.RemoveAPIService(apiServiceName)
	}
	delete(s.proxyHandlers, apiServiceName)

	// TODO unregister group level discovery when there are no more versions for the group
	// We don't need this right away because the handler properly delegates when no versions are present
}

// DefaultAPIResourceConfigSource returns default configuration for an APIResource.
func DefaultAPIResourceConfigSource() *serverstorage.ResourceConfig {
	ret := serverstorage.NewResourceConfig()
	// NOTE: GroupVersions listed here will be enabled by default. Don't put alpha versions in the list.
	ret.EnableVersions(
		v1.SchemeGroupVersion,
		v1beta1.SchemeGroupVersion,
	)

	return ret
}
