package service

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/wrappers"
)

type IdentityService struct {
}

func NewIdentityService() *IdentityService {
	return &IdentityService{}
}

func (i *IdentityService) GetPluginCapabilities(ctx context.Context, request *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	//TODO implement me
	capList := []csi.PluginCapability_Service_Type{
		csi.PluginCapability_Service_CONTROLLER_SERVICE,
		csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
	}
	var caps []*csi.PluginCapability
	for _, capobj := range capList {
		c := &csi.PluginCapability{
			Type: &csi.PluginCapability_Service_{
				Service: &csi.PluginCapability_Service{
					Type: capobj,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: caps,
	}, nil

}

func (i *IdentityService) Probe(ctx context.Context, request *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	status := wrappers.BoolValue{Value: true}
	//TODO implement me
	return &csi.ProbeResponse{
		Ready: &status,
	}, nil
}

var _ csi.IdentityServer = &IdentityService{}

func (i *IdentityService) GetPluginInfo(ctx context.Context, request *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	//TODO implement me
	return &csi.GetPluginInfoResponse{
		Name:          "mycsi.jtthink.com",
		VendorVersion: "v1.0",
	}, nil //kubectl get csinode xxx
}
