package service

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/utils/mount"
	"os"
	"strings"
)

type NodeService struct {
	nodeID  string
	mounter mount.Interface
}

func NewNodeService(nodeID string) *NodeService {
	return &NodeService{nodeID: nodeID, mounter: mount.New("")}
}

func (n *NodeService) NodeUnstageVolume(ctx context.Context, request *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (n *NodeService) NodePublishVolume(ctx context.Context, request *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	opts := request.GetVolumeCapability().GetMount().GetMountFlags()
	klog.Infoln("挂载参数：", opts)
	klog.Infof("NodePublishVolume")
	target := request.GetTargetPath()
	// 此时target 文件夹 格式是：
	// /var/lib/kubelet/pods/54408ec9-7843-4f36-99df-bbcd672406d7/volumes/kubernetes.io~csi/pvc-6cd86a79-9d92-464b-9a80-76ea620506f5/mount
	// 由于我们写的粗暴，这个文件夹是没有的，我们手工创建下
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
	//mount -t nfs 172.18.0.1:/home/nfsdata/pvc-6cd86a79-9d92-464b-9a80-76ea620506f5  /var/lib/kubelet/pods/54408ec9-7843-4f36-99df-bbcd672406d7/volumes/kubernetes.io~csi/pvc-6cd86a79-9d92-464b-9a80-76ea620506f5/mount
	vid := request.GetVolumeId()
	klog.Info("要挂载的volume是：", vid)
	pvName := strings.Replace(vid, "jtthink-volume-", "", -1)
	klog.Info("要挂载的pv是：", vid)
	err = n.mounter.Mount(basePath+"/"+pvName, target, "nfs", opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (n *NodeService) NodeUnpublishVolume(ctx context.Context, request *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	err := mount.CleanupMountPoint(request.GetTargetPath(), n.mounter, true)
	if err != nil {
		return nil, err
	}
	klog.Infof("NodeUnpublishVolume")
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (n *NodeService) NodeGetVolumeStats(ctx context.Context, request *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return &csi.NodeGetVolumeStatsResponse{}, nil
}

func (n *NodeService) NodeExpandVolume(ctx context.Context, request *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (n *NodeService) NodeGetCapabilities(ctx context.Context, request *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN, // 不执行 NodeStageVolume
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
	klog.Infoln("NodeGetInfo")
	return &csi.NodeGetInfoResponse{
		NodeId: n.nodeID,
	}, nil
}

var _ csi.NodeServer = &NodeService{}

// 如果使用云盘，
// 就会将云硬盘格式化成对应文件系统 将volume mount到一个全局的目录
func (n *NodeService) NodeStageVolume(ctx context.Context, request *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
