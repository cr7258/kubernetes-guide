package service

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/utils/mount"
	"os"
)

type NodeService struct {
	nodeID  string
	mounter mount.Interface
}

func NewNodeService(nodeID string) *NodeService {
	return &NodeService{nodeID: nodeID, mounter: mount.New("")}
}

func (n *NodeService) NodeUnstageVolume(ctx context.Context, request *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	//TODO implement me
	return nil, status.Error(codes.Unimplemented, "")
}

const FixedSourceDir = "172.19.0.8:/home/nfsdata"

func (n *NodeService) NodePublishVolume(ctx context.Context, request *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	opts := request.GetVolumeCapability().GetMount().GetMountFlags()
	klog.Infoln("挂载参数：", opts)
	//TODO implement me
	klog.Infof("NodePublishVolume")
	//mount -t nfs xxxx:xxx  /var/lib
	target := request.GetTargetPath()
	// 此时target 文件夹 格式是：
	//var/lib/kubelet/pods/cabd93de-cf9f-4025-a965-14c3d5cf1b8f/volumes/kubernetes.io~csi/pvc-c02b9101-0993
	// 由于 我们写的粗暴， 这个文件夹是没有的  。 我们手工创建下

	klog.Info("要挂载的目录是:", target)
	nn, err := n.mounter.IsLikelyNotMountPoint(target)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(target, 0755)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			nn = true
		}
	}
	if !nn {
		return &csi.NodePublishVolumeResponse{}, nil
	}
	//m := mount.New("")
	//m.Mount(FixedSourceDir, target, "nfs", opts)
	err = n.mounter.Mount(FixedSourceDir, target, "nfs", opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (n *NodeService) NodeUnpublishVolume(ctx context.Context, request *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	//TODO implement me

	err := mount.CleanupMountPoint(request.GetTargetPath(), n.mounter, true)
	if err != nil {
		return nil, err
	}
	klog.Infof("NodeUnpublishVolume")
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (n *NodeService) NodeGetVolumeStats(ctx context.Context, request *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	//TODO implement me
	return &csi.NodeGetVolumeStatsResponse{}, nil
}

func (n *NodeService) NodeExpandVolume(ctx context.Context, request *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	//TODO implement me
	return nil, status.Error(codes.Unimplemented, "")
}

func (n *NodeService) NodeGetCapabilities(ctx context.Context, request *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	//TODO implement me
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
					},
				},
			},
		},
	}, nil
}

func (n *NodeService) NodeGetInfo(ctx context.Context, request *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	//TODO implement me
	klog.Infoln("NodeGetInfo")
	return &csi.NodeGetInfoResponse{
		NodeId: n.nodeID,
	}, nil
}

var _ csi.NodeServer = &NodeService{}

// 如果使用云盘，
// 就会将云硬盘格式化成对应文件系统 将volume mount到一个全局的目录
func (n *NodeService) NodeStageVolume(ctx context.Context, request *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	//TODO implement me
	return nil, status.Error(codes.Unimplemented, "")
}
