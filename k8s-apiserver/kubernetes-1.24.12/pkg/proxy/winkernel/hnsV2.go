//go:build windows
// +build windows

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

package winkernel

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"

	"github.com/Microsoft/hcsshim/hcn"
	"k8s.io/klog/v2"

	"strings"
)

type hnsV2 struct{}

var (
	// LoadBalancerFlagsIPv6 enables IPV6.
	LoadBalancerFlagsIPv6 hcn.LoadBalancerFlags = 2
	// LoadBalancerPortMappingFlagsVipExternalIP enables VipExternalIP.
	LoadBalancerPortMappingFlagsVipExternalIP hcn.LoadBalancerPortMappingFlags = 16
)

func (hns hnsV2) getNetworkByName(name string) (*hnsNetworkInfo, error) {
	hnsnetwork, err := hcn.GetNetworkByName(name)
	if err != nil {
		klog.ErrorS(err, "Error getting network by name")
		return nil, err
	}

	var remoteSubnets []*remoteSubnetInfo
	for _, policy := range hnsnetwork.Policies {
		if policy.Type == hcn.RemoteSubnetRoute {
			policySettings := hcn.RemoteSubnetRoutePolicySetting{}
			err = json.Unmarshal(policy.Settings, &policySettings)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal Remote Subnet policy settings")
			}
			rs := &remoteSubnetInfo{
				destinationPrefix: policySettings.DestinationPrefix,
				isolationID:       policySettings.IsolationId,
				providerAddress:   policySettings.ProviderAddress,
				drMacAddress:      policySettings.DistributedRouterMacAddress,
			}
			remoteSubnets = append(remoteSubnets, rs)
		}
	}

	return &hnsNetworkInfo{
		id:            hnsnetwork.Id,
		name:          hnsnetwork.Name,
		networkType:   string(hnsnetwork.Type),
		remoteSubnets: remoteSubnets,
	}, nil
}

func (hns hnsV2) getAllEndpointsByNetwork(networkName string) (map[string]*(endpointsInfo), error) {
	hcnnetwork, err := hcn.GetNetworkByName(networkName)
	if err != nil {
		klog.ErrorS(err, "failed to get HNS network by name", "name", networkName)
		return nil, err
	}
	endpoints, err := hcn.ListEndpointsOfNetwork(hcnnetwork.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoints: %w", err)
	}
	endpointInfos := make(map[string]*(endpointsInfo))
	for _, ep := range endpoints {
		// Add to map with key endpoint ID or IP address
		// Storing this is expensive in terms of memory, however there is a bug in Windows Server 2019 that can cause two endpoints to be created with the same IP address.
		// TODO: Store by IP only and remove any lookups by endpoint ID.
		endpointInfos[ep.Id] = &endpointsInfo{
			ip:         ep.IpConfigurations[0].IpAddress,
			isLocal:    uint32(ep.Flags&hcn.EndpointFlagsRemoteEndpoint) == 0,
			macAddress: ep.MacAddress,
			hnsID:      ep.Id,
			hns:        hns,
			// only ready and not terminating endpoints were added to HNS
			ready:       true,
			serving:     true,
			terminating: false,
		}
		endpointInfos[ep.IpConfigurations[0].IpAddress] = endpointInfos[ep.Id]

		if len(ep.IpConfigurations) == 1 {
			continue
		}

		// If ipFamilyPolicy is RequireDualStack or PreferDualStack, then there will be 2 IPS (iPV4 and IPV6)
		// in the endpoint list
		endpointDualstack := &endpointsInfo{
			ip:         ep.IpConfigurations[1].IpAddress,
			isLocal:    uint32(ep.Flags&hcn.EndpointFlagsRemoteEndpoint) == 0,
			macAddress: ep.MacAddress,
			hnsID:      ep.Id,
			hns:        hns,
			// only ready and not terminating endpoints were added to HNS
			ready:       true,
			serving:     true,
			terminating: false,
		}
		endpointInfos[ep.IpConfigurations[1].IpAddress] = endpointDualstack
	}
	klog.V(3).InfoS("Queried endpoints from network", "network", networkName)
	klog.V(5).InfoS("Queried endpoints details", "network", networkName, "endpointInfos", endpointInfos)
	return endpointInfos, nil
}

func (hns hnsV2) getEndpointByID(id string) (*endpointsInfo, error) {
	hnsendpoint, err := hcn.GetEndpointByID(id)
	if err != nil {
		return nil, err
	}
	return &endpointsInfo{ //TODO: fill out PA
		ip:         hnsendpoint.IpConfigurations[0].IpAddress,
		isLocal:    uint32(hnsendpoint.Flags&hcn.EndpointFlagsRemoteEndpoint) == 0, //TODO: Change isLocal to isRemote
		macAddress: hnsendpoint.MacAddress,
		hnsID:      hnsendpoint.Id,
		hns:        hns,
	}, nil
}
func (hns hnsV2) getEndpointByIpAddress(ip string, networkName string) (*endpointsInfo, error) {
	hnsnetwork, err := hcn.GetNetworkByName(networkName)
	if err != nil {
		klog.ErrorS(err, "Error getting network by name")
		return nil, err
	}

	endpoints, err := hcn.ListEndpoints()
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoints: %w", err)
	}
	for _, endpoint := range endpoints {
		equal := false
		if endpoint.IpConfigurations != nil && len(endpoint.IpConfigurations) > 0 {
			equal = endpoint.IpConfigurations[0].IpAddress == ip

			if !equal && len(endpoint.IpConfigurations) > 1 {
				equal = endpoint.IpConfigurations[1].IpAddress == ip
			}
		}
		if equal && strings.EqualFold(endpoint.HostComputeNetwork, hnsnetwork.Id) {
			return &endpointsInfo{
				ip:         ip,
				isLocal:    uint32(endpoint.Flags&hcn.EndpointFlagsRemoteEndpoint) == 0, //TODO: Change isLocal to isRemote
				macAddress: endpoint.MacAddress,
				hnsID:      endpoint.Id,
				hns:        hns,
			}, nil
		}
	}
	return nil, fmt.Errorf("Endpoint %v not found on network %s", ip, networkName)
}
func (hns hnsV2) getEndpointByName(name string) (*endpointsInfo, error) {
	hnsendpoint, err := hcn.GetEndpointByName(name)
	if err != nil {
		return nil, err
	}
	return &endpointsInfo{ //TODO: fill out PA
		ip:         hnsendpoint.IpConfigurations[0].IpAddress,
		isLocal:    uint32(hnsendpoint.Flags&hcn.EndpointFlagsRemoteEndpoint) == 0, //TODO: Change isLocal to isRemote
		macAddress: hnsendpoint.MacAddress,
		hnsID:      hnsendpoint.Id,
		hns:        hns,
	}, nil
}
func (hns hnsV2) createEndpoint(ep *endpointsInfo, networkName string) (*endpointsInfo, error) {
	hnsNetwork, err := hcn.GetNetworkByName(networkName)
	if err != nil {
		return nil, err
	}
	var flags hcn.EndpointFlags
	if !ep.isLocal {
		flags |= hcn.EndpointFlagsRemoteEndpoint
	}
	ipConfig := &hcn.IpConfig{
		IpAddress: ep.ip,
	}
	hnsEndpoint := &hcn.HostComputeEndpoint{
		IpConfigurations: []hcn.IpConfig{*ipConfig},
		MacAddress:       ep.macAddress,
		Flags:            flags,
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
	}

	var createdEndpoint *hcn.HostComputeEndpoint
	if !ep.isLocal {
		if len(ep.providerAddress) != 0 {
			policySettings := hcn.ProviderAddressEndpointPolicySetting{
				ProviderAddress: ep.providerAddress,
			}
			policySettingsJson, err := json.Marshal(policySettings)
			if err != nil {
				return nil, fmt.Errorf("PA Policy creation failed: %v", err)
			}
			paPolicy := hcn.EndpointPolicy{
				Type:     hcn.NetworkProviderAddress,
				Settings: policySettingsJson,
			}
			hnsEndpoint.Policies = append(hnsEndpoint.Policies, paPolicy)
		}
		createdEndpoint, err = hnsNetwork.CreateRemoteEndpoint(hnsEndpoint)
		if err != nil {
			return nil, err
		}
	} else {
		createdEndpoint, err = hnsNetwork.CreateEndpoint(hnsEndpoint)
		if err != nil {
			return nil, err
		}
	}
	return &endpointsInfo{
		ip:              createdEndpoint.IpConfigurations[0].IpAddress,
		isLocal:         uint32(createdEndpoint.Flags&hcn.EndpointFlagsRemoteEndpoint) == 0,
		macAddress:      createdEndpoint.MacAddress,
		hnsID:           createdEndpoint.Id,
		providerAddress: ep.providerAddress, //TODO get from createdEndpoint
		hns:             hns,
	}, nil
}
func (hns hnsV2) deleteEndpoint(hnsID string) error {
	hnsendpoint, err := hcn.GetEndpointByID(hnsID)
	if err != nil {
		return err
	}
	err = hnsendpoint.Delete()
	if err == nil {
		klog.V(3).InfoS("Remote endpoint resource deleted", "hnsID", hnsID)
	}
	return err
}

// findLoadBalancerID will construct a id from the provided loadbalancer fields
func findLoadBalancerID(endpoints []endpointsInfo, vip string, protocol, internalPort, externalPort uint16) (loadBalancerIdentifier, error) {
	// Compute hash from backends (endpoint IDs)
	hash, err := hashEndpointInfos(endpoints)
	if err != nil {
		klog.V(2).ErrorS(err, "Error hashing endpoints", "endpoints", endpoints)
		return loadBalancerIdentifier{}, err
	}
	if len(vip) > 0 {
		return loadBalancerIdentifier{protocol: protocol, internalPort: internalPort, externalPort: externalPort, vip: vip, endpointsHash: hash}, nil
	}
	return loadBalancerIdentifier{protocol: protocol, internalPort: internalPort, externalPort: externalPort, endpointsHash: hash}, nil
}

func (hns hnsV2) getAllLoadBalancers() (map[loadBalancerIdentifier]*loadBalancerInfo, error) {
	lbs, err := hcn.ListLoadBalancers()
	var id loadBalancerIdentifier
	if err != nil {
		return nil, err
	}
	loadBalancers := make(map[loadBalancerIdentifier]*(loadBalancerInfo))
	for _, lb := range lbs {
		portMap := lb.PortMappings[0]
		// Compute hash from backends (endpoint IDs)
		hash, err := hashEndpointIds(lb.HostComputeEndpoints)
		if err != nil {
			klog.V(2).ErrorS(err, "Error hashing endpoints", "policy", lb)
			return nil, err
		}
		if len(lb.FrontendVIPs) == 0 {
			// Leave VIP uninitialized
			id = loadBalancerIdentifier{protocol: uint16(portMap.Protocol), internalPort: portMap.InternalPort, externalPort: portMap.ExternalPort, endpointsHash: hash}
		} else {
			id = loadBalancerIdentifier{protocol: uint16(portMap.Protocol), internalPort: portMap.InternalPort, externalPort: portMap.ExternalPort, vip: lb.FrontendVIPs[0], endpointsHash: hash}
		}
		loadBalancers[id] = &loadBalancerInfo{
			hnsID: lb.Id,
		}
	}
	klog.V(3).InfoS("Queried load balancers", "count", len(lbs))
	return loadBalancers, nil
}

func (hns hnsV2) getLoadBalancer(endpoints []endpointsInfo, flags loadBalancerFlags, sourceVip string, vip string, protocol uint16, internalPort uint16, externalPort uint16, previousLoadBalancers map[loadBalancerIdentifier]*loadBalancerInfo) (*loadBalancerInfo, error) {
	var id loadBalancerIdentifier
	vips := []string{}
	// Compute hash from backends (endpoint IDs)
	hash, err := hashEndpointInfos(endpoints)
	if err != nil || hash == [20]byte{} {
		klog.V(2).ErrorS(err, "Error hashing endpoints", "endpoints", endpoints)
		return nil, err
	}
	if len(vip) > 0 {
		id = loadBalancerIdentifier{protocol: protocol, internalPort: internalPort, externalPort: externalPort, vip: vip, endpointsHash: hash}
		vips = append(vips, vip)
	} else {
		id = loadBalancerIdentifier{protocol: protocol, internalPort: internalPort, externalPort: externalPort, endpointsHash: hash}
	}

	if lb, found := previousLoadBalancers[id]; found {
		klog.V(1).InfoS("Found cached Hns loadbalancer policy resource", "policies", lb)
		return lb, nil
	}

	lbPortMappingFlags := hcn.LoadBalancerPortMappingFlagsNone
	if flags.isILB {
		lbPortMappingFlags |= hcn.LoadBalancerPortMappingFlagsILB
	}
	if flags.useMUX {
		lbPortMappingFlags |= hcn.LoadBalancerPortMappingFlagsUseMux
	}
	if flags.preserveDIP {
		lbPortMappingFlags |= hcn.LoadBalancerPortMappingFlagsPreserveDIP
	}
	if flags.localRoutedVIP {
		lbPortMappingFlags |= hcn.LoadBalancerPortMappingFlagsLocalRoutedVIP
	}
	if flags.isVipExternalIP {
		lbPortMappingFlags |= LoadBalancerPortMappingFlagsVipExternalIP
	}

	lbFlags := hcn.LoadBalancerFlagsNone
	if flags.isDSR {
		lbFlags |= hcn.LoadBalancerFlagsDSR
	}

	if flags.isIPv6 {
		lbFlags |= LoadBalancerFlagsIPv6
	}

	lbDistributionType := hcn.LoadBalancerDistributionNone

	if flags.sessionAffinity {
		lbDistributionType = hcn.LoadBalancerDistributionSourceIP
	}

	loadBalancer := &hcn.HostComputeLoadBalancer{
		SourceVIP: sourceVip,
		PortMappings: []hcn.LoadBalancerPortMapping{
			{
				Protocol:         uint32(protocol),
				InternalPort:     internalPort,
				ExternalPort:     externalPort,
				DistributionType: lbDistributionType,
				Flags:            lbPortMappingFlags,
			},
		},
		FrontendVIPs: vips,
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
		Flags: lbFlags,
	}

	for _, ep := range endpoints {
		loadBalancer.HostComputeEndpoints = append(loadBalancer.HostComputeEndpoints, ep.hnsID)
	}

	lb, err := loadBalancer.Create()

	if err != nil {
		return nil, err
	}

	klog.V(1).InfoS("Created Hns loadbalancer policy resource", "loadBalancer", lb)
	lbInfo := &loadBalancerInfo{
		hnsID: lb.Id,
	}
	// Add to map of load balancers
	previousLoadBalancers[id] = lbInfo
	return lbInfo, err
}

func (hns hnsV2) deleteLoadBalancer(hnsID string) error {
	lb, err := hcn.GetLoadBalancerByID(hnsID)
	if err != nil {
		// Return silently
		return nil
	}

	err = lb.Delete()
	if err != nil {
		// There is a bug in Windows Server 2019, that can cause the delete call to fail sometimes. We retry one more time.
		// TODO: The logic in syncProxyRules  should be rewritten in the future to better stage and handle a call like this failing using the policyApplied fields.
		klog.V(1).ErrorS(err, "Error deleting Hns loadbalancer policy resource. Attempting one more time...", "loadBalancer", lb)
		return lb.Delete()
	}
	return err
}

func hashEndpointIds(endpoints []string) (hash [20]byte, err error) {
	hash = [20]byte{}
	for _, ep := range endpoints {
		hash, err = hashNextEndpoint(ep, hash)
		if err != nil {
			return [20]byte{}, err
		}
	}
	return
}

func hashEndpointInfos(endpoints []endpointsInfo) (hash [20]byte, err error) {
	var id string
	for _, ep := range endpoints {
		id = strings.ToUpper(ep.hnsID)
		hash, err = hashNextEndpoint(id, hash)
		if err != nil {
			return [20]byte{}, err
		}
	}
	return
}

func hashNextEndpoint(endpointId string, hashIn [20]byte) (hashOut [20]byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
			hashOut = [20]byte{}
		}
	}()

	if len(endpointId) > 0 {
		sha1 := sha1.Sum(([]byte(endpointId)))
		// We XOR the hashes of endpoints, since they are an unordered set.
		// This can cause collisions, but is sufficient since we are using other keys to identify the load balancer.
		hashOut = xor(hashIn, sha1)
	}
	return
}

func xor(b1 [20]byte, b2 [20]byte) (xorbytes [20]byte) {
	for i := 0; i < 20; i++ {
		xorbytes[i] = b1[i] ^ b2[i]
	}
	return xorbytes
}
